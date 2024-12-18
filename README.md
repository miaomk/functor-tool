# functor-tool
针对 Functor 项目写的一个自动 check in 奖励的脚本

---

## 🚀 功能

- 🌾 自动登录
- 💰 自动 `check in` 积分
---

## 💻 环境及需要的账户

- 安装 Golang 环境 (目前我用的Go版本是 `go 1.23.2`)
- 已经注册好的账号的 `email`,`password`

---

## 🛠️ 设置

1. 克隆仓库：
   ```bash
   git clone https://github.com/miaomk/functor-tool
   cd functor-tool
   ```
2. 安装Golang 环境：
   ```bash
    这个我就不多说了 网上都有教程
   ```

---

## ⚙️ 配置

### account.txt

该文件包含账号的设置：
```bash
email:password
```

### ip.txt
该文件包含ip的设置：

```bash
socks5://user:password@ip:port
http://user:password@ip:port
```




---

## 🚀 使用

1. 确保所有配置文件已正确设置。
2. 运行脚本：
   ```bash
    go mod tidy
    go run main.go
   ```
---
