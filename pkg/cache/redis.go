package cache

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Jidetireni/ara-cooperative/internal/config"
	"github.com/Jidetireni/ara-cooperative/pkg/logger"
	rds "github.com/redis/go-redis/v9"
)

type Redis struct {
	Client *rds.Client
	Logger *logger.Logger
}

var ErrCacheMiss = errors.New("cache miss")

func New(config *config.Config, logger *logger.Logger) (*Redis, func()) {
	ops, err := rds.ParseURL(config.Redis.URI)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to parse redis url")
	}

	redis := &Redis{
		Client: rds.NewClient(ops),
		Logger: logger,
	}

	err = redis.Ping()
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to ping redis")
	}

	cleanUp := func() {
		_ = redis.Close()
	}

	return redis, cleanUp
}

func (r *Redis) Ping() error {
	return r.Client.Ping(context.Background()).Err()
}

func (r *Redis) Close() error {
	return r.Client.Close()
}

func (r *Redis) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	v, err := json.Marshal(value)
	if err != nil {
		return err
	}

	r.Logger.Debug().Str("key", key).Msg("setting cache value")
	return r.Client.Set(ctx, key, v, expiration).Err()
}

func (r *Redis) Get(ctx context.Context, key string, dest any) error {
	r.Logger.Debug().Str("key", key).Msg("getting cache value")
	val, err := r.Client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, rds.Nil) {
			r.Logger.Debug().Str("key", key).Msg("cache miss")
			return ErrCacheMiss
		}
		return err
	}

	return json.Unmarshal([]byte(val), dest)
}

func (r *Redis) SetPrimitive(ctx context.Context, key string, value string, expiration time.Duration) error {
	r.Logger.Debug().Str("key", key).Msg("setting primitive cache value")
	return r.Client.Set(ctx, key, value, expiration).Err()
}

func (r *Redis) GetPrimitive(ctx context.Context, key string) (string, error) {
	r.Logger.Debug().Str("key", key).Msg("getting primitive cache value")
	val, err := r.Client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, rds.Nil) {
			r.Logger.Debug().Str("key", key).Msg("primitive cache miss")
			return "", ErrCacheMiss
		}
		return "", err
	}

	return val, nil
}

func (r *Redis) Delete(ctx context.Context, key string) error {
	r.Logger.Debug().Str("key", key).Msg("deleting cache value")
	return r.Client.Del(ctx, key).Err()
}
