package monitor

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis"
	"github.com/pkg/errors"
)

type StatusStore interface {
	GetStatus(t Target) (Status, bool, error)
	SetStatus(t Target, s Status, exp time.Duration) error
}

type RedisOptions struct {
	Host     string
	Port     uint
	Password string
	DB       int
}

func (ro RedisOptions) toPkgOptions() *redis.Options {
	host := ro.Host
	if host == "" {
		host = "localhost"
	}

	port := ro.Port
	if port == 0 {
		port = 6379
	}

	return &redis.Options{
		Addr:     fmt.Sprintf("%v:%v", host, port),
		Password: ro.Password,
		DB:       ro.DB,
	}
}

type RedisStore struct {
	client *redis.Client
}

var _ StatusStore = &RedisStore{}

func NewRedisStore(opt RedisOptions) *RedisStore {
	return &RedisStore{
		client: redis.NewClient(opt.toPkgOptions()),
	}
}

const redisKeyTemplate = "avamon_status_%v"

func targetToRedisKey(t Target) string {
	return fmt.Sprintf(redisKeyTemplate, t.ID)
}

type redisStatus struct {
	TID   uint          `json:"tid"`
	Title string        `json:"title"`
	URL   string        `json:"url"`
	Type  string        `json:"type"`
	Err   string        `json:"err"`
	Time  time.Duration `json:"time"`
	HTTP  int           `json:"http"`
}

func serializeStatusRedis(t Target, s Status) (string, error) {
	errMsg := ""
	if s.Err != nil {
		errMsg = s.Err.Error()
	}

	rs := redisStatus{
		TID:   t.ID,
		Title: t.Title,
		URL:   t.URL,
		Type:  s.Type.String(),
		Err:   errMsg,
		Time:  s.ResponseTime,
		HTTP:  s.HTTPStatusCode,
	}

	bs, err := json.Marshal(rs)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

func deserializeStatusRedis(s string) (Target, Status, error) {
	rs := redisStatus{}
	err := json.Unmarshal([]byte(s), &rs)
	if err != nil {
		return Target{}, Status{}, err
	}

	stype, ok := ScanStatusType(rs.Type)
	if !ok {
		return Target{}, Status{}, fmt.Errorf("Could no match status type %q", rs.Type)
	}

	target := Target{}
	target.ID = rs.TID
	target.Title = rs.Title
	target.URL = rs.URL

	status := Status{}
	status.Type = stype
	status.Err = fmt.Errorf("%s", rs.Err)
	status.ResponseTime = rs.Time
	status.HTTPStatusCode = rs.HTTP

	return target, status, nil
}

func (rs *RedisStore) Ping() error {
	err := rs.client.Ping().Err()
	if err != nil {
		return errors.Wrap(err, "ping failed")
	}
	return nil
}

func (rs *RedisStore) Scan() ([]TargetStatus, error) {
	match := fmt.Sprintf(redisKeyTemplate, "*")

	var cursor uint64
	tarstats := []TargetStatus{}
	for {
		var keys []string
		var err error
		keys, cursor, err = rs.client.Scan(cursor, match, 10).Result()
		if err != nil {
			return nil, err
		}

		for _, key := range keys {
			str, err := rs.client.Get(key).Result()
			if err != nil {
				continue
			}

			target, status, err := deserializeStatusRedis(str)
			if err != nil {
				continue
			}

			tarstats = append(tarstats, TargetStatus{target, status})
		}

		if cursor == 0 {
			break
		}
	}

	return tarstats, nil
}

func (rs *RedisStore) GetStatus(t Target) (Status, bool, error) {
	str, err := rs.client.Get(targetToRedisKey(t)).Result()
	if err == redis.Nil {
		return Status{}, false, nil
	}
	if err != nil {
		return Status{}, false, errors.Wrap(err, "could not get value from redis")
	}

	tcheck, status, err := deserializeStatusRedis(str)
	if err != nil {
		return Status{}, false, errors.Wrap(err, "could not deserialize status")
	}

	if tcheck.ID != t.ID || tcheck.URL != t.URL {
		return Status{}, false, errors.Errorf(
			"target validation failed: (actual != expected) %v != %v", tcheck, t)
	}

	return status, true, nil
}

func (rs *RedisStore) SetStatus(t Target, s Status, exp time.Duration) error {
	str, err := serializeStatusRedis(t, s)
	if err != nil {
		return errors.Wrap(err, "could not serialize status")
	}

	err = rs.client.Set(targetToRedisKey(t), str, exp).Err()
	if err != nil {
		return errors.Wrap(err, "could not set value on redis")
	}

	return nil
}
