package common

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

var RedisClient *redis.Client

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

func PublishConfigChange(ctx context.Context, channel string, message string) error {
	if RedisClient == nil {
		return nil
	}
	return RedisClient.Publish(ctx, channel, message).Err()
}
