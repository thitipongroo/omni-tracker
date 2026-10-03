package repository

import (
	"context"
	"github.com/redis/go-redis/v9"
)

var RDB *redis.Client
var Ctx = context.Background()

func InitRedis(url string) {
	RDB = redis.NewClient(&redis.Options{Addr: url})
}
