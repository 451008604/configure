package common

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisClient 全局 Redis 客户端实例
// 在 InitRedis 函数中初始化
var RedisClient *redis.Client

// InitRedis 初始化 Redis 连接
// 从环境变量读取配置：
//
//	REDIS_ADDR - Redis 服务器地址，默认 127.0.0.1:6379
//	REDIS_PASSWORD - Redis 密码，默认为空
//
// 连接后会执行 Ping 测试，验证连接是否成功
// 连接失败不会阻塞服务启动，只会记录错误日志
func InitRedis() {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}
	pass := os.Getenv("REDIS_PASSWORD")
	RedisClient = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: pass,
		DB:       0,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := RedisClient.Ping(ctx).Err(); err != nil {
		log.Printf("Redis connection failed: %v", err)
	} else {
		log.Println("Redis connected")
	}
}

// PublishConfigChange 向 Redis 频道发布配置变更通知
// 参数：
//
//	ctx - 上下文，用于控制超时
//	channel - Redis 频道名
//	message - 通知消息内容（JSON 格式）
//
// 如果 Redis 未连接，返回 nil 不报错
// 使用示例：
//
//	PublishConfigChange(ctx, "config_updates", `{"file":"base.json"}`)
func PublishConfigChange(ctx context.Context, channel string, message string) error {
	if RedisClient == nil {
		return nil
	}
	return RedisClient.Publish(ctx, channel, message).Err()
}
