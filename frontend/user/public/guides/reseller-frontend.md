# 分站前端开发手册

## 1. 两种方式

- 成品店面：分站控制台 → 站点配置 → 前端与外观，选择雪糕店面（xgai）、经典店面或 Vault，设置品牌色。Logo、站点名称、公告、导航、客服与页脚继续使用下方配置。
- 自研前端：上传静态构建结果 ZIP，预览草稿后再发布。支持 Vue、React、Vite 和纯 HTML；不支持需要 Node/PHP/Python 运行时或服务端渲染的源码项目。Nuxt/Next.js 请导出纯静态文件。

## 2. 打包规范与发布

1. 执行你项目自己的生产构建，例如 `npm run build`。
2. 把 dist（或 out）目录**内部文件**压成 ZIP；ZIP 根目录必须直接有 `index.html`，不能再套 dist、项目名等目录。
3. 上传“新草稿”。压缩包最多 10 MB，解压最多 25 MB，单文件最多 5 MB，最多 1000 个文件。允许 HTML、JS/MJS、CSS、JSON、SVG、常见图片和字体；拒绝路径穿越、符号链接、隐藏文件、可执行后端文件。不要上传 node_modules、.env、源码、密钥或 source map。
4. 先绑定并通过审核分站域名，再点击“预览草稿”。预览链接一小时有效，仅在你的分站域名下生效。预览受沙箱限制，不能登录、发起支付、提交表单或调用业务接口；这是外观预览，不是支付验收。
5. “发布草稿”原子切换线上版本；上传草稿不影响旧版本。保存于持久数据库，常规容器重建不会删除。
6. “恢复平台店面”关闭自研前端，保留上传草稿。恢复后按所选平台模板与品牌色展示。

不要注册 Service Worker。发布页面禁止 worker，避免恢复平台版本后仍被旧缓存接管。推荐相对资源路径；预览会重写压缩包内部的根路径资源引用，但不能自动修复任意路由器 basename、动态拼接 URL 或写死的线上域名。请使用 ZIP 示例验证。

## 3. 域名与权限

所有商城请求使用分站自己的同源 `/api/v1/...`。平台按请求域名识别分站，自动应用本分站上下架、售价和订单归属。不要把请求改到主站域名，不要伪造 Host，不要从浏览器调用上游采购 API。

前端不需要代理 Key、Secret、管理员密码或 HMAC 签名。它们绝不能放进 ZIP、前端环境变量、localStorage、日志或请求 URL。

`/api`、`/uploads`、`/health` 和系统后台路径由平台管理，不会被 ZIP 覆盖。`/auth`、`/me`、`/reseller`、`/guest`、`/pay`、`/cart`、`/checkout`、`/orders` 及其子路径保留平台页面，确保登录、查单与支付始终可用。

如果要从自定义商城进入平台商品详情，使用 `/products/{slug}?platform_storefront=1`。任意平台页面加 `platform_storefront=1` 可临时使用内置店面，不会改变发布状态。

分站后台仍在主站的分销控制台管理，分站前端脚本不会在主站后台执行。

## 4. 通用响应与公开接口

成功响应通常为 `{ "status_code": 0, "msg": "...", "data": ... }`，列表分页位于响应的 `pagination`。HTTP 200 不代表业务成功，必须检查 status_code。可用 `X-Lang: zh-CN / zh-TW / en-US` 选择语言。

```js
async function api(path, options = {}) {
  const response = await fetch('/api/v1' + path, options);
  const result = await response.json();
  if (!response.ok || result.status_code !== 0) throw new Error(result.msg || '请求失败');
  return result;
}
const {data: config} = await api('/public/config');
const {data: products, pagination} = await api('/public/products?page=1&page_size=20');
const {data: product} = await api('/public/products/' + encodeURIComponent(slug));
```

- `config.brand`：站点名称、Logo、站点 URL 等；`config.contact`：本站客服。
- `config.reseller_theme.primary_color`：品牌色（可能为空）；`config.storefront_template`：内置模板。
- `config.payment_channels`：公开支付渠道，不能包含密钥。
- 商品多语言标题通常为 `title` 对象；商品详情内 `skus` 提供规格。始终使用接口返回的现价、sku_id、可购状态和库存范围，不用硬编码价格或进价。
- `GET /public/categories`：分类。售罄、下架和金额限制均需按实时响应处理。

示例 ZIP 是商品展示与平台购买入口示例，默认只执行公开只读查询，不会自动创建订单。

## 5. 游客订单与支付

最省事的方式：自定义首页和商品展示，点击商品进入上述平台详情。平台负责规格、购物车、邮箱、订单密码、验证码与支付，避免自己维护整条资金流程。

如要自己接下单，按顺序使用：

1. `POST /guest/orders/preview`：邮箱、订单密码、items 等校验与报价。不要把返回预览当作已下单。
2. `POST /guest/orders`：创建真实订单。这个接口会产生真实订单，不要自动重试创建；超时后先查原单。
3. `POST /guest/payments`：为已有订单创建真实支付；传 `order_no`、`channel_id`，游客凭据放 Authorization。
4. 获取支付链接后跳转支付，或进入平台 `/pay?guest=1&order_no=...`。
5. 付款回跳后读取订单，只有平台订单状态确认已支付才展示支付成功。浏览器回跳参数、二维码出现、上游 submitted=true 都不能当作到账凭证。

订单预览/创建基础请求体：

```json
{
  "email": "customer@example.com",
  "order_password": "客户自己设置的订单密码",
  "items": [{"product_id": 123, "sku_id": 456, "quantity": 1}],
  "captcha_payload": {}
}
```

人工商品可能要求 `manual_form_data`；验证码配置以公开配置与平台原生结算实现为准，不可绕过。`create-and-pay` 是可选真实建单入口，不应用于只读测试。

游客查单与支付使用 `Authorization: Guest <token>`。token 为 `email.trim().toLowerCase() + '\n' + order_password.trim()` 的 UTF-8 字节经 base64url 编码，去掉末尾等号：

```js
function guestHeader(email, password) {
  const bytes = new TextEncoder().encode(email.trim().toLowerCase() + '\n' + password.trim());
  let binary = ''; bytes.forEach(b => binary += String.fromCharCode(b));
  return 'Guest ' + btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}
await api('/guest/orders/' + encodeURIComponent(orderNo), {
  headers: { Authorization: guestHeader(email, password) }
});
```

游客凭据是订单访问权限，只存当前标签页 sessionStorage，不放 query、长期 localStorage 或日志。要交给平台支付/查单页接管，可用当前标签页：

```js
sessionStorage.setItem('guest_order_auth', JSON.stringify({email, order_password: password}));
location.assign('/pay?guest=1&order_no=' + encodeURIComponent(orderNo));
```

查单接口：`GET /guest/orders`、`GET /guest/orders/{order_no}`。最新待付款记录：`GET /guest/payments/latest?order_no=...`，也需要 Guest Authorization。具体交付与退款以平台订单响应为准。

## 6. 已登录用户

推荐跳转平台 `/auth/login` 和 `/me`，自行开发时复用本站用户身份，访问 `/orders/...`、`/payments` 等用户接口时携带本站用户 Bearer token。管理员 token 与用户 token 不是一回事，不能混用。前端不要绕过邮箱验证、2FA、验证码或业务权限校验。

## 7. 上线自测

- 检查本站品牌与售价，不串主站或别人的站。
- 检查手机、日间/夜间模式、中文与其他语言，以及刷新深层页面、缺失图片。
- 确认平台详情、邮箱与订单密码、游客查单、登录和支付入口可进入。
- 支付需要另做受控的真实订单与到账验收，预览和公开接口成功不代表资金链路已测。
- 发布后确认页面资源已更新；有问题立即“恢复平台店面”。

## 分站核销入口

已验证且启用的分站域名自动获得验码、首次提交和进度查询权限，无需单独登记白名单。控制台显示专属链接 `https://redeem.xuegaogx.cc/?site=你的分站域名`，核销页使用分站名称、Logo、图标和颜色；未设置品牌图时沿用雪糕 AI。停用分站、解绑域名或域名未验证时拒绝访问。此入口共用核销服务，不会自动创建独立的核销子域名或 DNS 记录。

自建前端可从自己的已验证 HTTPS 域名调用 `https://redeem.xuegaogx.cc/api/v1/redemptions/{verify|submit|progress}`，使用 JSON POST 和 `credentials: omit`，不携带采购凭证。首次提交必须经过客户确认与授权。核销码是办理凭证，请勿在链接、日志或埋点里泄露。
