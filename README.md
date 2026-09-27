swmu晚寝签到自动化代码（后端）

使用Go开发，轻量化的卓越代表！

Go版本：1.24.4

数据库使用 PostgreSQL。首次运行前请在后端目录的 `.env` 中配置 PostgreSQL 连接信息：

```env
PGHOST=127.0.0.1
PGPORT=5432
PGUSER=postgres
PGPASSWORD=你的PostgreSQL密码
PGDATABASE=dormcheck
PGSSLMODE=disable
```

请先在 PostgreSQL 中创建 `dormcheck` 数据库，并确保该用户有建表和迁移表结构的权限。程序启动时会自动迁移表结构。PostgreSQL 连接参数必须留在 `.env`，因为应用需要先连接数据库才能读取设置；不要提交 `.env`。

验证码 AI、JWT、邮件服务、高德地图、用户绑定上限、密码与验证码策略等运行设置会保存在 PostgreSQL 的 `system_settings` 表中，并可在超级管理员后台调整和查看。设置接口仅向超级管理员返回这些密钥；高德 Web JS API 凭据会在登录后下发给地图组件使用，请在高德控制台限制允许域名。数据库中的密钥以数据库字段形式保存，请限制 PostgreSQL 账号权限并保护备份文件。

首次启动时，`JWT_SECRET`、`CAPTCHA_AI_API_KEY`（兼容旧变量 `DASHSCOPE_API_KEY`）、`AMAP_WEB_KEY`、`AMAP_SECURITY_JS_CODE` 和 `SMTP_*` 环境变量仅用于填充数据库中尚不存在的设置。确认设置写入数据库后，可从 `.env` 删除这些变量；`PG*` 连接参数必须保留。SMTP 凭据不再内置于代码，首次管理账号注册前可临时通过 `SMTP_USER`、`SMTP_PASSWORD` 配置邮件服务，或在已有超级管理员后台填写。

设置后台的 `/admin/settings` 仅超级管理员可读写。验证码 AI 支持兼容 OpenAI Chat Completions 的视觉接口，可设置接口地址、模型、密钥、提示词、超时和重试次数；用户绑定上限、邮箱验证码时效、密码策略及 JWT 有效期也可在后台调整。

任务页按签到活动 ID 汇总。学生账号可以绑定到多个平台用户；同一个学生的同一活动只有一条共享任务，修改、暂停、删除或手动签到都会影响所有绑定用户。每个用户只会看到自己绑定的学生任务，管理员可查看全平台活动和绑定用户。解绑时如果仍有任务，至少要保留一个用户绑定；最后一个绑定用户需要先删除该学生的活动任务。升级时若检测到旧的按用户重复任务，会合并为一条：保留 ID 最大记录的任务配置，并保留最近一次执行结果。活动学院、签到模式和活动时间段从微学工活动信息中随任务保存；旧任务没有这些历史字段时会显示“未记录”，编辑保存后会补齐。活动失败统计按最近 7 天计算，身份失效统计根据失败信息中的登录、Cookie、会话或令牌错误关键词识别。

部署前需要自行修改“自定义”项目，请对所有代码进行关键词“自定义”的搜索
并自行替换！
（各种平台key需要自己去申请！）

打包时请在linux环境下打包，否则无法架设到服务器！
使用 CGO_ENABLED=1 编译（推荐）
在 Linux 上编译时：
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o dormcheck main.go



后端默认监听8081端口。

用户角色：0管理员，1普通用户，2赞助用户，3超级管理员。首次启用管理后台后，可在 PostgreSQL 中将指定用户提升为超级管理员：

```sql
UPDATE users SET role = 3, token_version = token_version + 1 WHERE email = '管理员邮箱';
```

管理员和超级管理员都可查看用户并调整角色；管理员不能修改超级管理员或自己的角色。角色变更会要求目标用户重新登录。

代码均有注释，如有不懂，欢迎加群咨询！

DormCheck官网：https://dc.kikirepository.cn/
