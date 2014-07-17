package main

import (
	"fmt"
	"github.com/garyburd/redigo/redis"
	"github.com/go-martini/martini"
	"github.com/martini-contrib/cors"
	"github.com/soveran/redisurl"
	"os"
	"time"
)

type Server struct {
	Pool    *redis.Pool
	Martini *martini.ClassicMartini
}

func (server *Server) RedisAddress() string {
	redisAddress := os.Getenv("REDIS_URL")
	if redisAddress == "" {
		redisAddress = os.Getenv("REDISCLOUD_URL")
		if redisAddress == "" {
			redisAddress = "redis://localhost:6379"
		}
	}
	return redisAddress
}

func (server *Server) CreatePool() {
	server.Pool = &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			fmt.Println("Connecting to " + server.RedisAddress())
			return redisurl.ConnectToURL(server.RedisAddress())
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}
}

func (server *Server) Redis(commandName string, args ...interface{}) (reply interface{}, err error) {
	conn := server.Pool.Get()
	defer conn.Close()
	return conn.Do(commandName, args...)
}

func (server *Server) CreateMartini() {
	fmt.Println(&server)
	server.Martini = martini.Classic()
	server.Martini.Use(cors.Allow(&cors.Options{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET"},
		ExposeHeaders:    []string{"Content-Type, Cache-Control, Expires, Etag, Last-Modified"},
		AllowCredentials: true,
	}))
	server.Martini.Get("/score(\\.(?P<format>json|html|svg|txt))?", server.GetScore)
}

func (server *Server) Run() {
	fmt.Println(&server)
	server.Martini.Run()
}

func (server *Server) Start() {
	server.CreatePool()
	server.CreateMartini()
	server.Run()
}
