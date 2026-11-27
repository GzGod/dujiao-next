package risklimits

type Limits struct {
	FirstAmount    int64 `json:"first_amount"`
	SlowSeconds    int64 `json:"slow_seconds"`
	ShortMinutes   int64 `json:"short_minutes"`
	LongHours      int64 `json:"long_hours"`
	RepeatAmount   int64 `json:"repeat_amount"`
	ShortTotal     int64 `json:"short_total"`
	LongTotal      int64 `json:"long_total"`
	PaidCount      int64 `json:"paid_count"`
	ShortUnpaid    int64 `json:"short_unpaid"`
	LongUnpaid     int64 `json:"long_unpaid"`
	ReorderMinutes int64 `json:"reorder_minutes"`
	HighAmount     int64 `json:"high_amount"`
	LoyalDays      int64 `json:"loyal_days"`
	LoyalOrders    int64 `json:"loyal_orders"`
}

func (l Limits) Normalized() Limits {
	if l.FirstAmount <= 0 || l.FirstAmount > 1000000 {
		l.FirstAmount = 200
	}
	if l.SlowSeconds <= 0 || l.SlowSeconds > 1000000 {
		l.SlowSeconds = 150
	}
	if l.ShortMinutes <= 0 || l.ShortMinutes > 1000000 {
		l.ShortMinutes = 30
	}
	if l.LongHours <= 0 || l.LongHours > 720 {
		l.LongHours = 24
	}
	if l.RepeatAmount <= 0 || l.RepeatAmount > 1000000 {
		l.RepeatAmount = 150
	}
	if l.ShortTotal <= 0 || l.ShortTotal > 1000000 {
		l.ShortTotal = 300
	}
	if l.LongTotal <= 0 || l.LongTotal > 1000000 {
		l.LongTotal = 400
	}
	if l.PaidCount <= 0 || l.PaidCount > 1000000 {
		l.PaidCount = 3
	}
	if l.ShortUnpaid <= 0 || l.ShortUnpaid > 1000000 {
		l.ShortUnpaid = 3
	}
	if l.LongUnpaid <= 0 || l.LongUnpaid > 1000000 {
		l.LongUnpaid = 4
	}
	if l.ReorderMinutes <= 0 || l.ReorderMinutes > 1000000 {
		l.ReorderMinutes = 120
	}
	if l.HighAmount <= 0 || l.HighAmount > 1000000 {
		l.HighAmount = 150
	}
	if l.LoyalDays <= 0 || l.LoyalDays > 1000000 {
		l.LoyalDays = 7
	}
	if l.LoyalOrders <= 0 || l.LoyalOrders > 1000000 {
		l.LoyalOrders = 2
	}
	if l.ShortMinutes > l.LongHours*60 {
		l.ShortMinutes = l.LongHours * 60
	}
	if l.ReorderMinutes > l.LongHours*60 {
		l.ReorderMinutes = l.LongHours * 60
	}
	return l
}
