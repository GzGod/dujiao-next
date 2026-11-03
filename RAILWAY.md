# Railway 部署指南 — dujiao-next

## 必填环境变量

在 Railway 项目的 Variables 面板中设置这些变量（全部必填，否则容器拒绝启动）：

| 变量名 | 说明 |
|---|---|
| `APP_SECRET_KEY` | AES-256 加密密钥，≥32位随机字符串，与下面两个不同 |
| `JWT_SECRET` | 管理员 JWT 密钥，≥32位随机字符串 |
| `USER_JWT_SECRET` | 用户 JWT 密钥，≥32位随机字符串 |
| `DATABASE_URL` | Railway Postgres 插件自动注入，格式 `postgres://...` |

生成密钥：
```bash
openssl rand -hex 32
```
执行三次，分别填入上面三个密钥变量。

---

## Redis（可选但推荐）

添加 Railway Redis 插件后，设置：

| 变量名 | 说明 |
|---|---|
| `REDIS_URL` | Railway Redis 插件自动注入，格式 `redis://:password@host:port` |

不添加 Redis 时，将 `REDIS_ENABLED=false` 和 `QUEUE_ENABLED=false` 一并设置。

---

## 可选环境变量

| 变量名 | 默认值 | 说明 |
|---|---|---|
| `SERVER_MODE` | `release` | `debug` 或 `release` |
| `WEB_ADMIN_PATH` | `/admin` | 后台路径前缀，生产建议改随机字符串如 `/mgmt-x9k2` |
| `ADMIN_USERNAME` | 空（随机生成） | 首次启动的管理员用户名 |
| `ADMIN_PASSWORD` | 空（随机生成） | 首次启动的管理员密码（首次启动后从日志获取） |
| `EMAIL_ENABLED` | `false` | 开启邮件需同时设置 `EMAIL_HOST/PORT/USERNAME/PASSWORD/FROM` |
| `DB_MAX_OPEN_CONNS` | `25` | 数据库最大连接数 |
| `PORT` | `8080` | Railway 自动注入，无需手动设置 |

---

## 部署步骤

1. 在 Railway 新建项目，连接此 Git 仓库
2. 添加 **Postgres** 插件（`DATABASE_URL` 自动注入）
3. 添加 **Redis** 插件（`REDIS_URL` 自动注入）—— 可选
4. 在 Variables 中填写 `APP_SECRET_KEY`、`JWT_SECRET`、`USER_JWT_SECRET`
5. 触发部署；首次启动日志中会打印随机生成的管理员密码（如未设置 `ADMIN_PASSWORD`）

---

## 数据持久化

SQLite 在 Railway 的 ephemeral 文件系统上**不持久**，重新部署后数据丢失。
务必使用 **Railway Postgres 插件**（`DATABASE_URL` + `DATABASE_DRIVER=postgres`，entrypoint 已自动处理）。

uploads 目录同理：生产环境建议接入 S3 兼容对象存储（项目支持后续扩展）。
