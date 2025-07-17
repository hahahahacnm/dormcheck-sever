swmu晚寝签到自动化代码（后端）

使用Go开发，轻量化的卓越代表！

Go版本：1.24.4

部署前需要自行修改“自定义”项目，请对所有代码进行关键词“自定义”的搜索
并自行替换！
（各种平台key需要自己去申请！）

打包时请在linux环境下打包，否则无法架设到服务器！
使用 CGO_ENABLED=1 编译（推荐）
在 Linux 上编译时：
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o dormcheck main.go



架设时在服务器占用8080端口！

用户组权限（role）说明：0管理员，1普通用户，2赞助用户

代码均有注释，如有不懂，欢迎加群咨询！

DormCheck官网：https://dc.kikirepository.cn/
