# configure

**解决私有配置公开访问时的隐私泄露问题*

例如在GitHub中公开项目时，可能会导致数据库密码，证书文件等敏感数据公开，造成不可预知的风险。

## 特性

- 支持白名单配置（IPv4、域名）
- 支持对称加密，防止 MITM 攻击
- 支持https

## 使用

📌️ docker-compose 中设置环境变量 aesKey  
📌 如果需要 https 时，需要在 docker 容器中进行映射  
✔️ 白名单配置在[whitelist.txt](whitelist.txt)  
获取配置文件内容可参考 client 中的[示例](client%2Fmain.go)
