package gormstore

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"gorm.io/gorm/logger"
	"sort"
	"strings"
	"time"

	"github.com/dujiao-next/internal/constants"
	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	ordercontract "github.com/dujiao-next/internal/modules/order/contract"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	orderriskdomain "github.com/dujiao-next/internal/modules/orderrisk/domain"
	"github.com/dujiao-next/internal/persistence/gormutil"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Store 是订单与退款记录端口的 GORM 实现。
type Store struct {
	db                    *gorm.DB
	guestCredentialSecret []byte
}

var _ ordercontract.Store = (*Store)(nil)

const guestCredentialHashPrefix = "hmac-sha256:"

// New 创建订单存储。访客凭据密钥是强制依赖，禁止退化为明文存储。
func New(db *gorm.DB, guestCredentialSecret string) *Store {
	secret := strings.TrimSpace(guestCredentialSecret)
	if secret == "" {
		panic("order store: guest credential secret is required")
	}
	return &Store{db: db, guestCredentialSecret: []byte(secret)}
}

func (r *Store) withChildren(query *gorm.DB) *gorm.DB {
	return query.
		Where("orders.deleted_at IS NULL").
		Preload("Items", "deleted_at IS NULL").
		Preload("Fulfillment", "deleted_at IS NULL").
		Preload("Children", "deleted_at IS NULL").
		Preload("Children.Items", "deleted_at IS NULL").
		Preload("Children.Fulfillment", "deleted_at IS NULL")
}

// Create 创建订单与订单项
func (r *Store) Create(order *orderdomain.Order, items []orderdomain.OrderItem) error {
	if order != nil && order.UserID == 0 && strings.TrimSpace(order.GuestPassword) != "" {
		if len(r.guestCredentialSecret) == 0 {
			return errors.New("guest credential secret is required")
		}
		order.GuestPassword = r.hashGuestCredential(order.GuestEmail, order.GuestPassword)
	}
	if err := r.db.Create(order).Error; err != nil {
		return err
	}
	for i := range items {
		items[i].OrderID = order.ID
	}
	if len(items) > 0 {
		if err := r.db.Create(&items).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *Store) hashGuestCredential(email, password string) string {
	if len(r.guestCredentialSecret) == 0 {
		panic("order store: guest credential secret is required")
	}
	mac := hmac.New(sha256.New, r.guestCredentialSecret)
	_, _ = mac.Write([]byte(strings.ToLower(strings.TrimSpace(email))))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(password))
	return guestCredentialHashPrefix + hex.EncodeToString(mac.Sum(nil))
}

func isGuestCredentialHash(value string) bool {
	if !strings.HasPrefix(value, guestCredentialHashPrefix) {
		return false
	}
	encoded := strings.TrimPrefix(value, guestCredentialHashPrefix)
	if len(encoded) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(encoded)
	return err == nil
}

// BackfillGuestCredentialHashes 把历史游客订单明文凭证迁移为不可逆的密钥化摘要。
// 应用密钥缺失时拒绝执行，避免启动后继续以明文模式运行。
func (r *Store) BackfillGuestCredentialHashes() (int64, error) {
	if len(r.guestCredentialSecret) == 0 {
		return 0, errors.New("guest credential secret is required")
	}
	type guestCredentialRow struct {
		ID            uint
		GuestEmail    string
		GuestPassword string
	}
	const batchSize = 500
	var migrated int64
	var lastID uint
	candidateSQL, candidateArgs := guestCredentialBackfillCandidateSQL(r.db.Dialector.Name())
	for {
		var rows []guestCredentialRow
		if err := r.db.Model(&orderdomain.Order{}).
			Select("id", "guest_email", "guest_password").
			Where("user_id = 0 AND guest_password <> '' AND id > ?", lastID).
			Where(candidateSQL, candidateArgs...).
			Order("id ASC").
			Limit(batchSize).
			Find(&rows).Error; err != nil {
			return migrated, err
		}
		if len(rows) == 0 {
			return migrated, nil
		}

		err := r.db.Transaction(func(tx *gorm.DB) error {
			for _, row := range rows {
				if isGuestCredentialHash(row.GuestPassword) {
					continue
				}
				result := tx.Model(&orderdomain.Order{}).
					Where("id = ? AND guest_password = ?", row.ID, row.GuestPassword).
					UpdateColumn("guest_password", r.hashGuestCredential(row.GuestEmail, row.GuestPassword))
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					return fmt.Errorf("guest credential migration lost update for order %d", row.ID)
				}
				migrated += result.RowsAffected
			}
			return nil
		})
		if err != nil {
			return migrated, err
		}
		lastID = rows[len(rows)-1].ID
	}
}

// guestCredentialBackfillCandidateSQL 保证数据库候选集不比
// isGuestCredentialHash 的严格判定更窄。SQLite 与 PostgreSQL 使用各自的
// 字符类语法筛出含非十六进制字符的伪摘要；未知方言宁可分批扫描全部游客凭据，
// 也不能把形似摘要的历史明文永久排除在迁移之外。
func guestCredentialBackfillCandidateSQL(dialect string) (string, []interface{}) {
	prefixPattern := guestCredentialHashPrefix + "%"
	expectedLength := len(guestCredentialHashPrefix) + sha256.Size*2
	hashStart := len(guestCredentialHashPrefix) + 1
	// 长度与起始位置是编译期常量，直接内联成 SQL 字面量：PostgreSQL 在
	// SUBSTRING(x FROM $n) 中会把未知类型的占位符按 substring(text, text)
	// 正则重载推断为 text，pgx 随后无法把整数编码成 text 而报错。
	switch strings.ToLower(strings.TrimSpace(dialect)) {
	case "sqlite":
		return fmt.Sprintf("(guest_password NOT LIKE ? OR LENGTH(guest_password) <> %d OR SUBSTR(guest_password, %d) GLOB ?)", expectedLength, hashStart),
			[]interface{}{prefixPattern, "*[^0-9A-Fa-f]*"}
	case "postgres":
		return fmt.Sprintf("(guest_password NOT LIKE ? OR LENGTH(guest_password) <> %d OR SUBSTRING(guest_password FROM %d) !~ ?)", expectedLength, hashStart),
			[]interface{}{prefixPattern, "^[0-9A-Fa-f]+$"}
	default:
		return "1 = 1", nil
	}
}

// GetByID 根据 ID 获取订单
func (r *Store) GetByID(id uint) (*orderdomain.Order, error) {
	var order orderdomain.Order
	query := r.withChildren(r.db)
	if err := query.First(&order, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &order, nil
}

// GetByIDs 根据 ID 列表批量获取订单
func (r *Store) GetByIDs(ids []uint) ([]orderdomain.Order, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var orders []orderdomain.Order
	if err := r.db.Where("orders.deleted_at IS NULL AND id IN ?", ids).Find(&orders).Error; err != nil {
		return nil, err
	}
	return orders, nil
}

// ResolveReceiverEmailByOrderID 根据订单 ID 解析状态通知的收件邮箱。
func (r *Store) ResolveReceiverEmailByOrderID(orderID uint) (string, error) {
	if orderID == 0 {
		return "", nil
	}

	var orderRow struct {
		UserID     uint
		GuestEmail string
	}
	if err := r.db.Model(&orderdomain.Order{}).
		Select("user_id", "guest_email").
		Where("orders.deleted_at IS NULL AND id = ?", orderID).
		Take(&orderRow).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	if orderRow.UserID == 0 {
		return strings.TrimSpace(orderRow.GuestEmail), nil
	}

	var userRow struct {
		Email string
	}
	if err := r.db.Model(&userdomain.User{}).
		Select("email").
		Where("id = ? AND deleted_at IS NULL", orderRow.UserID).
		Take(&userRow).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(userRow.Email), nil
}

// GetByIDAndUser 获取用户订单详情
func (r *Store) GetByIDAndUser(id uint, userID uint) (*orderdomain.Order, error) {
	var order orderdomain.Order
	query := r.withChildren(r.db)
	if err := query.Where("id = ? AND user_id = ? AND parent_id IS NULL", id, userID).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &order, nil
}
func (r *Store) GetByOrderNoAndUser(orderNo string, userID uint) (*orderdomain.Order, error) {
	var order orderdomain.Order
	query := r.withChildren(r.db)
	if err := query.Where("order_no = ? AND user_id = ? AND parent_id IS NULL", orderNo, userID).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &order, nil
}

// GetAnyByOrderNoAndUser 按订单号查找订单（不限父/子），用于交付下载等场景
func (r *Store) GetAnyByOrderNoAndUser(orderNo string, userID uint) (*orderdomain.Order, error) {
	var order orderdomain.Order
	query := r.withChildren(r.db)
	if err := query.Where("order_no = ? AND user_id = ?", orderNo, userID).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &order, nil
}

// GetAnyByOrderNoAndGuest 按订单号查找游客订单（不限父/子），用于交付下载等场景
func (r *Store) GetAnyByOrderNoAndGuest(orderNo, email, password string) (*orderdomain.Order, error) {
	var order orderdomain.Order
	query := r.withChildren(r.db)
	if err := query.Where("order_no = ? AND user_id = 0 AND guest_email = ? AND guest_password = ?", orderNo, email, r.hashGuestCredential(email, password)).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &order, nil
}

// GetByIDAndGuest 获取游客订单详情
func (r *Store) GetByIDAndGuest(id uint, email, password string) (*orderdomain.Order, error) {
	var order orderdomain.Order
	query := r.withChildren(r.db)
	if err := query.
		Where("id = ? AND user_id = 0 AND guest_email = ? AND guest_password = ? AND parent_id IS NULL", id, email, r.hashGuestCredential(email, password)).
		First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &order, nil
}

// GetByOrderNoAndGuest 获取游客订单详情（按订单号）
func (r *Store) GetByOrderNoAndGuest(orderNo, email, password string) (*orderdomain.Order, error) {
	var order orderdomain.Order
	query := r.withChildren(r.db)
	if err := query.
		Where("order_no = ? AND user_id = 0 AND guest_email = ? AND guest_password = ? AND parent_id IS NULL", orderNo, email, r.hashGuestCredential(email, password)).
		First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &order, nil
}

func applyTenantScope(query *gorm.DB, scope ordercontract.TenantScope) *gorm.DB {
	if scope.ResellerID == nil {
		return query.Where("orders.reseller_id IS NULL")
	}
	return query.Where("orders.reseller_id = ?", *scope.ResellerID)
}

// GetByIDAndUserScoped 获取用户订单详情，并强制限定当前前台租户范围。
func (r *Store) GetByIDAndUserScoped(id uint, userID uint, scope ordercontract.TenantScope) (*orderdomain.Order, error) {
	var order orderdomain.Order
	query := r.withChildren(r.db)
	query = applyTenantScope(query.Where("id = ? AND user_id = ? AND parent_id IS NULL", id, userID), scope)
	if err := query.First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &order, nil
}

// GetByOrderNoAndUserScoped 按订单号获取用户订单详情，并强制限定当前前台租户范围。
func (r *Store) GetByOrderNoAndUserScoped(orderNo string, userID uint, scope ordercontract.TenantScope) (*orderdomain.Order, error) {
	var order orderdomain.Order
	query := r.withChildren(r.db)
	query = applyTenantScope(query.Where("order_no = ? AND user_id = ? AND parent_id IS NULL", orderNo, userID), scope)
	if err := query.First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &order, nil
}

// GetAnyByOrderNoAndUserScoped 按订单号查找用户订单（不限父/子），并强制限定当前前台租户范围。
func (r *Store) GetAnyByOrderNoAndUserScoped(orderNo string, userID uint, scope ordercontract.TenantScope) (*orderdomain.Order, error) {
	var order orderdomain.Order
	query := r.withChildren(r.db)
	query = applyTenantScope(query.Where("order_no = ? AND user_id = ?", orderNo, userID), scope)
	query = downloadableOrder(query)
	if err := query.First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &order, nil
}

// GetByIDAndGuestScoped 获取游客订单详情，并强制限定当前前台租户范围。
func (r *Store) GetByIDAndGuestScoped(id uint, email, password string, scope ordercontract.TenantScope) (*orderdomain.Order, error) {
	var order orderdomain.Order
	query := r.withChildren(r.db)
	query = applyTenantScope(query.Where("id = ? AND user_id = 0 AND guest_email = ? AND guest_password = ? AND parent_id IS NULL", id, email, r.hashGuestCredential(email, password)), scope)
	if err := query.First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &order, nil
}

// GetByOrderNoAndGuestScoped 获取游客订单详情（按订单号），并强制限定当前前台租户范围。
func (r *Store) GetByOrderNoAndGuestScoped(orderNo, email, password string, scope ordercontract.TenantScope) (*orderdomain.Order, error) {
	var order orderdomain.Order
	query := r.withChildren(r.db)
	query = applyTenantScope(query.Where("order_no = ? AND user_id = 0 AND guest_email = ? AND guest_password = ? AND parent_id IS NULL", orderNo, email, r.hashGuestCredential(email, password)), scope)
	if err := query.First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &order, nil
}

// GetAnyByOrderNoAndGuestScoped 按订单号查找游客订单（不限父/子），并强制限定当前前台租户范围。
func (r *Store) GetAnyByOrderNoAndGuestScoped(orderNo, email, password string, scope ordercontract.TenantScope) (*orderdomain.Order, error) {
	var order orderdomain.Order
	query := r.withChildren(r.db)
	query = applyTenantScope(query.Where("order_no = ? AND user_id = 0 AND guest_email = ? AND guest_password = ?", orderNo, email, r.hashGuestCredential(email, password)), scope)
	query = downloadableOrder(query)
	if err := query.First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &order, nil
}

// downloadableOrder denies direct child downloads if the parent is missing
// or blocks fulfillment.
func downloadableOrder(query *gorm.DB) *gorm.DB {
	return query.Where(`orders.risk_hold_status IN ('', 'released') AND
		(orders.parent_id IS NULL OR EXISTS (
			SELECT 1 FROM orders AS parent WHERE parent.id = orders.parent_id
			AND parent.deleted_at IS NULL AND parent.risk_hold_status IN ('', 'released')
		))`)
}

// ListChildren 获取子订单列表
func (r *Store) ListChildren(parentID uint) ([]orderdomain.Order, error) {
	var orders []orderdomain.Order
	if parentID == 0 {
		return orders, nil
	}
	if err := r.withChildren(r.db).
		Where("parent_id = ?", parentID).
		Order("id asc").
		Find(&orders).Error; err != nil {
		return nil, err
	}
	return orders, nil
}

// ListAdmin 管理端订单列表
func (r *Store) ListAdmin(filter ordercontract.ListFilter) ([]orderdomain.Order, int64, error) {
	var orders []orderdomain.Order
	query := r.db.Model(&orderdomain.Order{}).Where("orders.deleted_at IS NULL AND parent_id IS NULL")
	if filter.PendingExpiredBefore != nil {
		query = query.Where("orders.status = ? AND orders.paid_at IS NULL AND orders.expires_at <= ?",
			constants.OrderStatusPendingPayment, *filter.PendingExpiredBefore)
	}
	if filter.RiskAlertDue != nil {
		query = query.Where("orders.risk_hold_status = ? AND orders.risk_alert_retry_at <= ?", "pending_review", *filter.RiskAlertDue)
	}
	if filter.RiskReleaseRecoveryDue != nil {
		query = query.Where("orders.risk_hold_status = ? AND orders.risk_release_recovery_at <= ?", "released", *filter.RiskReleaseRecoveryDue)
	}
	if filter.RiskHoldStatus != "" {
		query = query.Where("orders.risk_hold_status = ?", filter.RiskHoldStatus)
	}
	if filter.RiskEvaluatedOnly {
		query = query.Where("orders.payment_risk_evaluated_at IS NOT NULL")
	}
	if filter.RiskShadowCandidate {
		query = query.Where("orders.payment_risk_candidate = ? AND orders.risk_hold_status = ?", true, "")
	}
	if filter.ExcludeID != 0 {
		query = query.Where("orders.id <> ?", filter.ExcludeID)
	}
	if filter.RiskDeviceHash != "" {
		query = query.Where("orders.risk_device_hash = ?", filter.RiskDeviceHash)
	}
	if filter.RiskIP != "" {
		query = query.Where("orders.risk_ip = ?", filter.RiskIP)
	}
	if filter.RiskIPv4Prefix != "" {
		query = query.Where("orders.risk_ip LIKE ?", filter.RiskIPv4Prefix+"%")
	}
	if filter.RiskActivityFrom != nil {
		query = query.Where("(orders.paid_at >= ? OR (orders.paid_at IS NULL AND orders.created_at >= ?))",
			*filter.RiskActivityFrom, *filter.RiskActivityFrom)
	}
	if filter.PaidFrom != nil {
		query = query.Where("orders.paid_at >= ?", *filter.PaidFrom)
	}
	if filter.PaidOnly {
		query = query.Where("orders.paid_at IS NOT NULL")
	}
	if filter.RiskSuccessfulOnly {
		query = query.Where("orders.paid_at IS NOT NULL AND orders.status IN ? AND orders.risk_hold_status IN ?",
			[]string{constants.OrderStatusDelivered, constants.OrderStatusCompleted}, []string{"", "released"})
	}
	if filter.IssuedOnly {
		// Select parent orders once even when several children have been delivered.
		query = query.Where(`EXISTS (
   SELECT 1 FROM fulfillments f JOIN orders issued ON issued.id = f.order_id
   WHERE f.deleted_at IS NULL AND f.status = ? AND issued.deleted_at IS NULL
    AND (issued.id = orders.id OR issued.parent_id = orders.id)
  )`, constants.FulfillmentStatusDelivered)
	}

	if filter.UserID != 0 {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if keyword := strings.TrimSpace(filter.UserKeyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where(
			"user_id IN ("+
				"SELECT users.id FROM users "+
				"WHERE users.deleted_at IS NULL AND ("+
				"users.email LIKE ? OR "+
				"users.display_name LIKE ? OR "+
				"EXISTS ("+
				"SELECT 1 FROM user_oauth_identities "+
				"WHERE user_oauth_identities.user_id = users.id AND ("+
				"user_oauth_identities.provider LIKE ? OR "+
				"user_oauth_identities.provider_user_id LIKE ? OR "+
				"user_oauth_identities.username LIKE ?"+
				")"+
				")"+
				")"+
				")",
			like, like, like, like, like,
		)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.OrderNo != "" {
		query = query.Where("order_no = ?", filter.OrderNo)
	}
	if filter.RiskContactHash != "" && filter.GuestEmail != "" {
		query = query.Where("(orders.risk_contact_hash = ? OR orders.guest_email = ?) AND orders.user_id = 0", filter.RiskContactHash, filter.GuestEmail)
	} else if filter.GuestEmail != "" {
		query = query.Where("guest_email = ?", filter.GuestEmail)
	}
	if keyword := strings.TrimSpace(filter.ProductKeyword); keyword != "" {
		like := "%" + keyword + "%"
		cond1, argCount1 := gormutil.BuildLocalizedLikeCondition(r.db, nil, []string{"oi.title_json"})
		cond2, argCount2 := gormutil.BuildLocalizedLikeCondition(r.db, nil, []string{"oi2.title_json"})
		if cond1 != "" {
			args1 := gormutil.RepeatLikeArgs(like, argCount1)
			args2 := gormutil.RepeatLikeArgs(like, argCount2)
			query = query.Where(
				"id IN (SELECT DISTINCT oi.order_id FROM order_items oi WHERE oi.deleted_at IS NULL AND oi.order_id IN (SELECT o2.id FROM orders o2 WHERE o2.deleted_at IS NULL AND o2.parent_id IS NULL) AND ("+cond1+")) "+
					"OR id IN (SELECT DISTINCT o3.parent_id FROM orders o3 WHERE o3.deleted_at IS NULL AND o3.parent_id IS NOT NULL AND o3.id IN (SELECT DISTINCT oi2.order_id FROM order_items oi2 WHERE oi2.deleted_at IS NULL AND "+cond2+"))",
				append(args1, args2...)...,
			)
		}
	}
	if filter.CreatedFrom != nil {
		query = query.Where("created_at >= ?", *filter.CreatedFrom)
	}
	if filter.CreatedTo != nil {
		query = query.Where("created_at <= ?", *filter.CreatedTo)
	}

	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	dataQuery := gormutil.ApplyPagination(query.Session(&gorm.Session{}), filter.Page, filter.PageSize)
	if !filter.SkipRelations {
		dataQuery = r.withChildren(dataQuery)
	}

	orderClause := resolveAdminOrderSort(filter.SortBy, filter.SortOrder)
	if filter.RiskAlertDue != nil {
		orderClause = "orders.risk_alert_retry_at ASC, orders.id ASC"
	} else if filter.RiskReleaseRecoveryDue != nil {
		orderClause = "orders.risk_release_recovery_at ASC, orders.id ASC"
	} else if filter.PendingExpiredBefore != nil {
		orderClause = "orders.expires_at ASC, orders.id ASC"
	} else if filter.RiskActivityFrom != nil {
		orderClause = "COALESCE(orders.paid_at, orders.created_at) DESC, orders.id DESC"
	}
	if err := dataQuery.Order(orderClause).Find(&orders).Error; err != nil {
		return nil, 0, err
	}
	return orders, total, nil
}

// resolveAdminOrderSort 解析排序参数，返回安全的 ORDER BY 子句。
func resolveAdminOrderSort(sortBy, sortOrder string) string {
	allowedColumns := map[string]bool{
		"created_at":   true,
		"updated_at":   true,
		"total_amount": true,
	}
	direction := "desc"
	if strings.ToLower(strings.TrimSpace(sortOrder)) == "asc" {
		direction = "asc"
	}
	col := strings.ToLower(strings.TrimSpace(sortBy))
	if !allowedColumns[col] {
		return "id " + direction
	}
	return col + " " + direction
}

// UpdateStatus 更新订单状态
func (r *Store) UpdateStatus(id uint, status string, updates map[string]interface{}) error {
	if updates == nil {
		updates = map[string]interface{}{}
	}
	updates["status"] = status
	return r.updateDeliveryProtectedFields(id, updates)
}

// ListByUser 获取用户订单列表
func (r *Store) ListByUser(filter ordercontract.ListFilter) ([]orderdomain.Order, int64, error) {
	var orders []orderdomain.Order
	query := r.db.Model(&orderdomain.Order{}).Where("orders.deleted_at IS NULL AND user_id = ? AND parent_id IS NULL", filter.UserID)

	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.OrderNo != "" {
		query = query.Where("order_no LIKE ?", "%"+filter.OrderNo+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = gormutil.ApplyPagination(query, filter.Page, filter.PageSize)

	query = r.withChildren(query)
	if err := query.Order("id desc").Find(&orders).Error; err != nil {
		return nil, 0, err
	}
	return orders, total, nil
}

// StatsByUser 按状态聚合用户订单数量（忽略分页与状态筛选，复用关键词筛选）
func (r *Store) StatsByUser(filter ordercontract.ListFilter) (map[string]int64, error) {
	query := r.db.Model(&orderdomain.Order{}).Where("orders.deleted_at IS NULL AND user_id = ? AND parent_id IS NULL", filter.UserID)
	// 注意：不应用 filter.Status，聚合目的就是看各状态分布
	if filter.OrderNo != "" {
		query = query.Where("order_no LIKE ?", "%"+filter.OrderNo+"%")
	}

	type row struct {
		Status string
		Count  int64
	}
	var rows []row
	if err := query.Select("status, COUNT(*) as count").Group("status").Scan(&rows).Error; err != nil {
		return nil, err
	}

	result := make(map[string]int64, len(rows))
	for _, item := range rows {
		result[item.Status] = item.Count
	}
	return result, nil
}

// ListByUserScoped 获取用户订单列表，并强制限定当前前台租户范围。
func (r *Store) ListByUserScoped(filter ordercontract.ListFilter, scope ordercontract.TenantScope) ([]orderdomain.Order, int64, error) {
	var orders []orderdomain.Order
	query := r.db.Model(&orderdomain.Order{}).Where("orders.deleted_at IS NULL AND user_id = ? AND parent_id IS NULL", filter.UserID)
	query = applyTenantScope(query, scope)

	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.OrderNo != "" {
		query = query.Where("order_no LIKE ?", "%"+filter.OrderNo+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = gormutil.ApplyPagination(query, filter.Page, filter.PageSize)
	query = r.withChildren(query)
	if err := query.Order("id desc").Find(&orders).Error; err != nil {
		return nil, 0, err
	}
	return orders, total, nil
}

// StatsByUserScoped 按状态聚合用户订单数量，并强制限定当前前台租户范围。
func (r *Store) StatsByUserScoped(filter ordercontract.ListFilter, scope ordercontract.TenantScope) (map[string]int64, error) {
	query := r.db.Model(&orderdomain.Order{}).Where("orders.deleted_at IS NULL AND user_id = ? AND parent_id IS NULL", filter.UserID)
	query = applyTenantScope(query, scope)
	if filter.OrderNo != "" {
		query = query.Where("order_no LIKE ?", "%"+filter.OrderNo+"%")
	}

	type row struct {
		Status string
		Count  int64
	}
	var rows []row
	if err := query.Select("status, COUNT(*) as count").Group("status").Scan(&rows).Error; err != nil {
		return nil, err
	}

	result := make(map[string]int64, len(rows))
	for _, item := range rows {
		result[item.Status] = item.Count
	}
	return result, nil
}

// ListByGuest 获取游客订单列表
func (r *Store) ListByGuest(email, password string, page, pageSize int) ([]orderdomain.Order, int64, error) {
	credentialHash := r.hashGuestCredential(email, password)
	var total int64
	if err := r.db.Model(&orderdomain.Order{}).
		Where("orders.deleted_at IS NULL AND user_id = 0 AND guest_email = ? AND guest_password = ? AND parent_id IS NULL", email, credentialHash).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var orders []orderdomain.Order
	query := r.withChildren(r.db)
	if err := query.
		Where("user_id = 0 AND guest_email = ? AND guest_password = ? AND parent_id IS NULL", email, credentialHash).
		Order("id desc").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&orders).Error; err != nil {
		return nil, 0, err
	}
	return orders, total, nil
}

// ListByGuestScoped 获取游客订单列表，并强制限定当前前台租户范围。
func (r *Store) ListByGuestScoped(email, password string, page, pageSize int, scope ordercontract.TenantScope) ([]orderdomain.Order, int64, error) {
	base := r.db.Model(&orderdomain.Order{}).Where("orders.deleted_at IS NULL AND user_id = 0 AND guest_email = ? AND guest_password = ? AND parent_id IS NULL", email, r.hashGuestCredential(email, password))
	base = applyTenantScope(base, scope)

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var orders []orderdomain.Order
	query := r.withChildren(base.Session(&gorm.Session{}))
	if err := query.
		Order("id desc").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&orders).Error; err != nil {
		return nil, 0, err
	}
	return orders, total, nil
}

// CountPendingByUserID 统计用户待支付的父订单数量
func (r *Store) CountPendingByUserID(userID uint) (int64, error) {
	if userID == 0 {
		return 0, nil
	}
	var count int64
	if err := r.db.Model(&orderdomain.Order{}).
		Where("orders.deleted_at IS NULL AND user_id = ? AND status = ? AND parent_id IS NULL", userID, constants.OrderStatusPendingPayment).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// LockRiskKeys 在当前事务中按固定顺序锁定风控维度；数据库只保存 SHA-256 摘要。
func (r *Store) LockRiskKeys(keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	hashes := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		digest := sha256.Sum256([]byte(key))
		hash := hex.EncodeToString(digest[:])
		if _, exists := seen[hash]; exists {
			continue
		}
		seen[hash] = struct{}{}
		hashes = append(hashes, hash)
	}
	sort.Strings(hashes)
	for _, hash := range hashes {
		row := orderriskdomain.LockKey{KeyHash: hash}
		if err := r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		if err := r.db.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("key_hash = ?", hash).
			First(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

// CountPendingGuestByRiskIP 统计某风控 IP 的游客待支付父订单数量。
func (r *Store) CountPendingGuestByRiskIP(riskIP string) (int64, error) {
	if riskIP == "" {
		return 0, nil
	}
	var count int64
	if err := r.db.Model(&orderdomain.Order{}).
		Where("orders.deleted_at IS NULL AND risk_ip = ? AND user_id = 0 AND status = ? AND parent_id IS NULL", riskIP, constants.OrderStatusPendingPayment).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountPendingMemberByRiskIP 统计某风控 IP 的会员待支付父订单数量。
func (r *Store) CountPendingMemberByRiskIP(riskIP string) (int64, error) {
	if riskIP == "" {
		return 0, nil
	}
	var count int64
	if err := r.db.Model(&orderdomain.Order{}).
		Where("orders.deleted_at IS NULL AND risk_ip = ? AND user_id > 0 AND status = ? AND parent_id IS NULL", riskIP, constants.OrderStatusPendingPayment).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// SumPendingGuestQuantityByRiskIP 按商品汇总某风控 IP 已锁定的游客待支付数量。
func (r *Store) SumPendingGuestQuantityByRiskIP(riskIP string, productIDs []uint) (map[uint]int64, error) {
	result := make(map[uint]int64, len(productIDs))
	if riskIP == "" || len(productIDs) == 0 {
		return result, nil
	}
	type quantityRow struct {
		ProductID uint
		Quantity  int64
	}
	var rows []quantityRow
	err := r.db.Table("order_items AS oi").
		Select("oi.product_id, COALESCE(SUM(oi.quantity), 0) AS quantity").
		Joins("JOIN orders AS child ON child.id = oi.order_id AND child.deleted_at IS NULL").
		Joins("JOIN orders AS parent ON parent.id = child.parent_id AND parent.deleted_at IS NULL").
		Where("oi.deleted_at IS NULL AND parent.risk_ip = ? AND parent.user_id = 0", riskIP).
		Where("parent.status = ? AND child.status = ?", constants.OrderStatusPendingPayment, constants.OrderStatusPendingPayment).
		Where("oi.product_id IN ?", productIDs).
		Group("oi.product_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.ProductID] = row.Quantity
	}
	return result, nil
}

// CountOrderItemsByProduct 统计商品关联的订单项数量
func (r *Store) CountOrderItemsByProduct(productID uint) (int64, error) {
	if productID == 0 {
		return 0, errors.New("invalid product id")
	}
	var count int64
	if err := r.db.Model(&orderdomain.OrderItem{}).Where("deleted_at IS NULL AND product_id = ?", productID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// UpdateFields 通用字段更新(供事务内/外使用,无 status 校验逻辑)。
// 配合 WithTx 使用以保证事务内写操作走 repo 层,不破坏 service-repo 分层。
func (r *Store) UpdateFields(id uint, updates map[string]interface{}) error {
	if id == 0 || len(updates) == 0 {
		return nil
	}
	return r.updateDeliveryProtectedFields(id, updates)
}

// UpdateChildrenStatus 把所有非目标状态的子订单批量更新为 targetStatus,返回受影响行数。
// targetStatus 为空字符串时直接返回 (0, nil) 不做任何更新。
func (r *Store) UpdateChildrenStatus(parentID uint, targetStatus string, now time.Time) (int64, error) {
	if parentID == 0 || strings.TrimSpace(targetStatus) == "" {
		return 0, nil
	}
	result := r.db.Model(&orderdomain.Order{}).
		Where("orders.deleted_at IS NULL AND parent_id = ? AND status <> ?", parentID, targetStatus).
		Updates(map[string]interface{}{
			"status":     targetStatus,
			"updated_at": now,
		})
	return result.RowsAffected, result.Error
}

// UpdateFieldsWhereWalletPaid 仅当订单 wallet_paid_amount > 0 时才更新指定字段,
// 返回受影响行数。用于 ReleaseOrderBalance 这类"已扣过余额才允许退回"的乐观锁场景。
func (r *Store) UpdateFieldsWhereWalletPaid(id uint, updates map[string]interface{}) (int64, error) {
	if id == 0 || len(updates) == 0 {
		return 0, nil
	}
	result := r.db.Model(&orderdomain.Order{}).
		Where("orders.deleted_at IS NULL AND id = ? AND wallet_paid_amount > 0", id).
		Updates(updates)
	return result.RowsAffected, result.Error
}

// GetByIDForUpdate 在事务中使用 SELECT ... FOR UPDATE 加行锁后读取订单,
// 不存在返回 (nil, nil)。SQLite 上 clause.Locking 是 no-op,PostgreSQL 上是真锁。
func (r *Store) GetByIDForUpdate(id uint) (*orderdomain.Order, error) {
	if id == 0 {
		return nil, nil
	}
	var order orderdomain.Order
	if err := r.db.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("orders.deleted_at IS NULL").
		First(&order, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &order, nil
}

// GetByIDForUpdateWithChildren 同 GetByIDForUpdate,并 Preload Items / Children / Children.Items,
// 用于支付/退款流程需要随父订单加载子订单的场景。
func (r *Store) GetByIDForUpdateWithChildren(id uint) (*orderdomain.Order, error) {
	if id == 0 {
		return nil, nil
	}
	var order orderdomain.Order
	err := r.db.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("orders.deleted_at IS NULL").
		Preload("Items", "deleted_at IS NULL").
		Preload("Children", "deleted_at IS NULL").
		Preload("Children.Items", "deleted_at IS NULL").
		First(&order, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &order, nil
}

// updateDeliveryProtectedFields serializes status/refund writes with Telegram
// delivery reservations. Any failure rolls back the caller's refund transaction.
func (r *Store) updateDeliveryProtectedFields(id uint, updates map[string]interface{}) error {
	_, status := updates["status"]
	_, refund := updates["refunded_amount"]
	if !status && !refund {
		return r.db.Model(&orderdomain.Order{}).Where("id = ? AND deleted_at IS NULL", id).Updates(updates).Error
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		var initial orderdomain.Order
		if err := tx.Where("id = ? AND deleted_at IS NULL", id).First(&initial).Error; err != nil {
			return err
		}
		root := id
		if initial.ParentID != nil {
			root = *initial.ParentID
		}
		var locked orderdomain.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", root).First(&locked).Error; err != nil {
			return err
		}
		var pending int64
		scope := root
		target, _ := updates["status"].(string)
		query := tx.Table("fulfillments").Where("deleted_at IS NULL AND status = ?", "pending")
		if !refund && (target == "delivered" || target == "completed" || target == "fulfilling") {
			scope = id
			query = query.Where("order_id = ?", scope)
		} else {
			query = query.Where("order_id = ? OR order_id IN (SELECT id FROM orders WHERE parent_id = ? AND deleted_at IS NULL)", scope, scope)
		}
		if err := query.Count(&pending).Error; err != nil {
			return err
		}
		if pending > 0 {
			return errors.New("order has a Telegram delivery in progress; review before changing status or refunding")
		}
		return tx.Model(&orderdomain.Order{}).Where("id = ? AND deleted_at IS NULL", id).Updates(updates).Error
	})
}

// RedemptionRiskBlocked covers historical and newly issued codes. Only
// deliveries linked to held orders are read, and payloads never leave the store.
func (s *Store) RedemptionRiskBlocked(ctx context.Context, digest [sha256.Size]byte) (bool, error) {
	rows, err := s.db.WithContext(ctx).Table("fulfillments AS f").Select("COALESCE(f.payload, ''), COALESCE(f.redemption_previous_hashes, '')").
		Joins("JOIN orders AS child ON child.id = f.order_id").
		Joins("LEFT JOIN orders AS parent ON parent.id = child.parent_id").
		Where("f.deleted_at IS NULL").
		Where("(child.risk_hold_status NOT IN ('', 'released') OR parent.risk_hold_status NOT IN ('', 'released'))").
		Rows()
	if err != nil {
		return false, err
	}
	defer rows.Close()
	encoded := hex.EncodeToString(digest[:])
	for rows.Next() {
		var payload, archived string
		if err := rows.Scan(&payload, &archived); err != nil {
			return false, err
		}
		for _, hash := range strings.Fields(archived) {
			if hash == encoded {
				return true, nil
			}
		}
		for _, line := range strings.Split(payload, "\n") {
			code := strings.TrimSpace(line)
			if len(code) >= 24 && len(code) <= 80 && sha256.Sum256([]byte(code)) == digest {
				return true, nil
			}
		}
	}
	return false, rows.Err()
}

// HasUnusedRedemption checks signed upstream observations on root or split
// deliveries. An untracked delivery does not count as confirmed unused.
func (s *Store) HasUnusedRedemption(orderIDs []uint) (bool, error) {
	if len(orderIDs) == 0 {
		return false, nil
	}
	var match uint
	err := s.db.Table("fulfillments AS f").Select("f.id").
		Joins("JOIN orders AS o ON o.id = f.order_id").
		Where("f.deleted_at IS NULL AND o.deleted_at IS NULL AND f.redemption_progress = ?", "unused").
		Where("(o.id IN ? OR o.parent_id IN ?)", orderIDs, orderIDs).
		Limit(1).Scan(&match).Error
	return match != 0, err
}

// RecordRedemptionSubmission clears a single-code delivery observation. The
// payload comparison prevents an old submission clearing a replacement.
func (s *Store) RecordRedemptionSubmission(ctx context.Context, digest [sha256.Size]byte) error {
	type delivery struct {
		ID      uint
		Payload string
	}
	db := s.db.WithContext(ctx).Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	query := db.Table("fulfillments AS f").Select("f.id, f.payload").
		Joins("JOIN orders AS child ON child.id = f.order_id").
		Joins("LEFT JOIN orders AS parent ON parent.id = child.parent_id").
		Where("f.deleted_at IS NULL AND f.redemption_progress = ?", "unused").
		Where("COALESCE(parent.paid_at, child.paid_at) >= ?", time.Now().Add(-2*time.Hour))
	var deliveries []delivery
	if err := query.Scan(&deliveries).Error; err != nil {
		return err
	}
	for _, delivered := range deliveries {
		code := strings.TrimSpace(delivered.Payload)
		if len(code) < 24 || len(code) > 80 || strings.ContainsAny(code, "\r\n") || sha256.Sum256([]byte(code)) != digest {
			continue
		}
		if err := db.Table("fulfillments").
			Where("id = ? AND deleted_at IS NULL AND payload = ? AND redemption_progress = ?", delivered.ID, delivered.Payload, "unused").
			UpdateColumn("redemption_progress", "no_longer_unused").Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) RecordRedemptionEvidence(ctx context.Context, evidence orderriskdomain.RedemptionEvidence) error {
	return s.db.WithContext(ctx).Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "event_id"}}, DoNothing: true}).Create(&evidence).Error
}

func (s *Store) ListRedemptionEvidence(ctx context.Context, codeHash, orderNo string, page, size int) ([]orderriskdomain.RedemptionEvidence, int64, error) {
	items := make([]orderriskdomain.RedemptionEvidence, 0)
	query := s.db.WithContext(ctx).Model(&orderriskdomain.RedemptionEvidence{})
	if codeHash != "" {
		query = query.Where("code_hash = ?", codeHash)
	}
	if orderNo != "" {
		var deliveries []struct {
			Payload                  string
			RedemptionPreviousHashes string
		}
		err := s.db.WithContext(ctx).Table("fulfillments AS f").Select("f.payload, f.redemption_previous_hashes").
			Joins("JOIN orders AS child ON child.id = f.order_id AND child.deleted_at IS NULL").
			Joins("LEFT JOIN orders AS parent ON parent.id = child.parent_id AND parent.deleted_at IS NULL").
			Where("f.deleted_at IS NULL AND (child.order_no = ? OR parent.order_no = ?)", orderNo, orderNo).
			Scan(&deliveries).Error
		if err != nil {
			return nil, 0, err
		}
		hashes := make([]string, 0, len(deliveries))
		for _, delivery := range deliveries {
			hashes = append(hashes, orderriskdomain.RedemptionPayloadHashes(delivery.Payload)...)
			hashes = append(hashes, strings.Fields(delivery.RedemptionPreviousHashes)...)
		}
		if len(hashes) == 0 {
			return items, 0, nil
		}
		query = query.Where("code_hash IN ?", hashes)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&items).Error
	return items, total, err
}
