package main

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/golang-jwt/jwt"
)

// Config 配置
type Config struct {
	Accounts []Account `mapstructure:"accounts"`
}

// Account 结构体系
type Account struct {
	AccessToken   string `mapstructure:"access_token"`
	UserId        string `mapstructure:"user_id"`
	Email         string `mapstructure:"email"`
	Password      string `mapstructure:"password"`
	NextClaimTime string `mapstructure:"next_claim_time"`
}

// LoadAccount 加载 account 配置文件
func LoadAccount() []Account {

	// 读取 account 文件信息
	file, err := os.Open("account.txt")

	if err != nil {
		panic(fmt.Errorf("账号文件不存在: [%s]", err))
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)

	var accounts []Account

	for scanner.Scan() {

		array := strings.Split(scanner.Text(), ":")

		if len(array) == 2 {

			accounts = append(accounts, Account{
				Email:    array[0],
				Password: array[1],
			})
		}
	}

	return accounts
}

// LoadProxyIps 加载 ip 配置文件
func LoadProxyIps() []string {

	// 读取 ip 文件信息
	file, err := os.Open("ip.txt")

	if err != nil {
		panic(fmt.Errorf("ip 文件不存在: [%s]", err))
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)

	var ipList []string

	for scanner.Scan() {

		ipList = append(ipList, scanner.Text())

	}

	return ipList
}

func BuildRequest(proxyUrl string) *resty.Request {

	requestHeaders := commonHeaders

	data, err := json.Marshal(requestHeaders)
	if err != nil {
		panic(fmt.Errorf("序列化解析 payload 失败: %v", err))
	}

	requestHeaders["Content-Length"] = fmt.Sprintf("%d", len(data))

	client := resty.New().
		SetBaseURL(baseUrl).
		SetRetryCount(3).
		SetTimeout(requestTimeout).
		SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true}).
		SetTransport(&http.Transport{
			MaxIdleConns:        5,                // 最大空闲连接数
			MaxIdleConnsPerHost: 10,               // 每个主机的最大空闲连接数
			IdleConnTimeout:     90 * time.Second, // 空闲连接超时时间
		})

	if proxyUrl != "" {
		client.SetProxy(proxyUrl)
	}

	return client.R().SetHeaders(requestHeaders)
}

// ExecuteMethod 执行 HTTP 请求
func ExecuteMethod(path, method, proxyUrl, token string, body map[string]string) (map[string]interface{}, error) {

	var response *resty.Response
	var err error

	request := BuildRequest(proxyUrl)

	if len(body) > 0 {
		request = request.SetBody(body)
	}
	if token != "" {
		request = request.SetAuthToken(token)
	}

	response, err = request.Execute(method, path)

	if err != nil {
		return nil, fmt.Errorf("发送[%s]请求失败: [%v]", path, err)
	}

	res, _ := json.Marshal(response.Body())
	if response.StatusCode() != 200 {
		return nil, fmt.Errorf("[%s]接口请求失败: [%v]", method, res)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(response.Body(), &result); err != nil {
		return nil, fmt.Errorf("解析响应数据失败: %v", err)
	}

	return result, nil
}

// EarnReward 领取奖励
func EarnReward(userId, proxyUrl, accessToken string) error {

	path := fmt.Sprintf("/api/v1/users/earn/%s", userId)

	_, err := ExecuteMethod(path, "GET", proxyUrl, accessToken, map[string]string{})
	if err != nil {
		return err
	}

	return nil
}

// UserLogin 获取 Token
func UserLogin(account *Account, url string) (string, error) {

	body := map[string]string{
		"email":    account.Email,
		"password": account.Password,
	}

	result, err := ExecuteMethod("/api/v1/auth/signin-user", "POST", url, account.AccessToken, body)

	if err != nil {
		return "", err
	}

	accessToken, ok1 := result["accessToken"].(string)

	if !ok1 {
		return "", fmt.Errorf("获取 token 请求响应数据格式错误")
	}

	return accessToken, nil
}

// GetUserInfo 获取用户信息
func GetUserInfo(proxyUrl string, account *Account) (string, string, float64, error) {
	result, err := ExecuteMethod("/api/v1/users", "GET", proxyUrl, account.AccessToken, map[string]string{})
	if err != nil {
		return "", "", 0, err
	}

	id, ok1 := result["id"].(string)
	dipTokenBalance, ok2 := result["dipTokenBalance"].(float64)
	dipInitMineTime, ok3 := result["dipInitMineTime"].(string)

	if !ok1 || !ok2 || !ok3 {
		return "", "", 0, fmt.Errorf("获取 token 请求响应数据格式错误")
	}

	return id, dipInitMineTime, dipTokenBalance, nil
}

// CheckJwtTokenExpiration 检查 access_token | refresh_token 是否已经过期
func CheckJwtTokenExpiration(tokenString string) (bool, error) {

	// 解析 Token 不验证签名
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})

	if err != nil {
		return false, fmt.Errorf("解析 jwt token 失败: [%v]", err)
	}

	// 从 Claims 中获取 `exp` 字段
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return false, fmt.Errorf("jwt token 转换 claims 类型失败")
	}

	exp, ok := claims["exp"].(float64)
	if !ok {
		return false, fmt.Errorf("jwt token 中没有 exp 字段或格式错误")
	}

	// 比较 `exp` 时间戳和当前时间
	expirationTime := time.Unix(int64(exp), 0)

	if time.Now().After(expirationTime) {
		return false, nil // 已过期
	}
	return true, nil // 未过期
}

func CheckUserToken(proxyUrl string, account *Account, buffer *strings.Builder) error {

	if account.AccessToken != "" {
		// access_token 过期则 login 获取最新 access_token | refresh_token
		if valid, err := CheckJwtTokenExpiration(account.AccessToken); err == nil && valid {
			return nil
		}
	}

	// token 过期或者没有则通过登录获取
	buffer.WriteString(fmt.Sprintf("开始获取 [%s] token\n", account.Email))

	newAccessToken, err := UserLogin(account, proxyUrl)
	if err != nil {
		buffer.WriteString(fmt.Sprintf("获取 [%s] token 失败,错误提示为: [%v]", account.Email, err))
		return err
	}

	// 更新 account 的 token 信息
	account.AccessToken = newAccessToken
	return nil
}

func CompareTime2Claim(proxyUrl string, account *Account, buffer *strings.Builder) {

	// 查询用户信息并获取奖励
	id, lastClaimTime, balance, err := GetUserInfo(proxyUrl, account)
	if err != nil {
		buffer.WriteString(fmt.Sprintf("获取 [%s] 用户信息失败,错误提示为: [%v]", account.Email, err))
		return
	}

	if lastClaimTime == "" {
		return
	}

	parseTime, err := time.Parse(time.RFC3339Nano, lastClaimTime)
	if err != nil {
		buffer.WriteString(fmt.Sprintf("解析时间出错:[%s]", err))
		return
	}

	// 获取当前时间
	now := time.Now()

	// 获取经过 24 小时后可以领取的时间
	nextClaimTime := parseTime.Add(time.Duration(24) * time.Hour)

	// 判断是否到时间了
	if now.Before(nextClaimTime) {

		diff := nextClaimTime.Sub(now).Minutes()

		buffer.WriteString(fmt.Sprintf("用户 [%s] 还差 [%v] 分钟可领取下一轮奖励 [%v]", account.Email, diff, balance))
		if account.UserId == "" {
			account.UserId = id
		}
		return
	}

	err = EarnReward(id, proxyUrl, account.AccessToken)

	if err != nil {
		buffer.WriteString(fmt.Sprintf("[%s] 领取奖励失败,error:[%v]", account.Email, err))
		return
	}
	buffer.WriteString(fmt.Sprintf("[%s] 奖励领取成功", account.Email))

	// 下一次领取的时间
	account.NextClaimTime = time.Now().Add(time.Duration(24) * time.Hour).Format("2006-01-02 15:04:05")

}

// ProcessAccount 每个账户单独的处理逻辑
func ProcessAccount(proxyUrl string, account *Account, buffer *strings.Builder) {

	err := CheckUserToken(proxyUrl, account, buffer)

	if err != nil {
		return
	}

	CompareTime2Claim(proxyUrl, account, buffer)
}

// StartLogWorker 日志处理
func StartLogWorker() {
	go func() {
		for logMsg := range logChannel {
			log.Println(logMsg) // 按顺序输出日志
		}
	}()
}

const (
	apiHost         = "node.securitylabs.xyz"
	baseUrl         = "https://node.securitylabs.xyz"
	requestInterval = 30 * time.Second
	requestTimeout  = 30 * time.Second
	//proxyUrl        = "http://127.0.0.1:7890"
)

var ips []string
var logChannel = make(chan string, 1000) // 日志队列 容量为1000 //
var commonHeaders = map[string]string{
	"Content-Type": "application/json",
	"User-Agent":   "PostmanRuntime/7.29.0",
	"Host":         apiHost,
	"Accept":       "*/*",
} // 全局 http 请求头

func main() {

	// 加载账号信息 与 全局请求 client
	accounts := LoadAccount()
	ips := LoadProxyIps()

	amount := len(accounts)
	if amount == 0 {
		fmt.Println("没有账户需要处理")
		return
	}

	// 启动日志处理器
	StartLogWorker()

	for {

		var wg sync.WaitGroup

		logChannel <- fmt.Sprintf("开始 [%d] 个账号的处理", amount)

		// 日志缓存 用于按照账号顺序进行日志打印
		logCache := make([]string, amount)

		for index := range accounts {
			wg.Add(1)

			account := &accounts[index]

			go func(index int) {
				defer wg.Done()
				var logBuffer strings.Builder

				if len(ips) < index {
					ips[index] = ""
				}

				ProcessAccount(ips[index], account, &logBuffer)
				// 存储日志
				logCache[index] = logBuffer.String()
			}(index)
		}
		wg.Wait()

		// 按照顺序输出日志
		for _, logMsg := range logCache {
			logChannel <- logMsg
		}

		logChannel <- fmt.Sprintf("[%v] 个账户处理完毕", amount)
		logChannel <- fmt.Sprint("----------------------------------\n")

		time.Sleep(requestInterval)
	}

	// 关闭日志通道（在程序退出时）
	close(logChannel)
}
