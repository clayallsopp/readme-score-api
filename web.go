package main

import (
    "github.com/go-martini/martini"
    "net/http"
    "encoding/json"
    "os/exec"
    "fmt"
    "strings"
    "github.com/garyburd/redigo/redis"
    "time"
)

// Expire caches in an hour
const CACHE_TTL = 60 * 60

type Server struct {
    Redis *redis.Conn
    Martini *martini.ClassicMartini
}

type ScoreResponse struct {
    Score string `json:"score"`
    URL string `json:"url"`
}

type ErrorResponse struct {
    Error string `json:"error"`
}

func MarshalToJsonBytes(res interface{}) []byte {
    resAsJson, _ := json.Marshal(res)
    return ([]byte(resAsJson))
}

func GetScoreResponseAsJson(score string, url_or_slug string) []byte {
    res := &ScoreResponse{
        Score:   score,
        URL: url_or_slug}
    return MarshalToJsonBytes(res)
}

func GetScoreErrorAsJson(url_or_slug string) []byte {
    res := &ErrorResponse{
        Error: "Could not determine score for " + url_or_slug}
    return MarshalToJsonBytes(res)
}

func CacheKeyForUrlOrSlug(url_or_slug string) string{
    return "url_or_slug:" + url_or_slug
}

func (server *Server) GetScore(res http.ResponseWriter, req *http.Request, params martini.Params) {
    res.Header().Set("Content-Type", "application/json")

    query_params := req.URL.Query()

    var url_or_slug string
    var param_matches []string
    var ok bool
    if param_matches,ok = query_params["url"]; !ok {
        param_matches = query_params["github"]
    }
    url_or_slug = param_matches[0]

    score, score_error := server.GetScoreForUrlOrSlug(url_or_slug)

    if score_error != nil {
        res.Write(GetScoreErrorAsJson(url_or_slug))
    } else {
        res.Write(GetScoreResponseAsJson(score, url_or_slug))
    }

}

func (server *Server) GetCachedScoreForUrlOrSlug(url_or_slug string) (string, error) {
    score, err := redis.String((*server.Redis).Do("GET", CacheKeyForUrlOrSlug(url_or_slug)))
    return score, err
}

func (server *Server)  CacheScoreForUrlOrSlug(score string, url_or_slug string) {
    (*server.Redis).Do("SET", CacheKeyForUrlOrSlug(url_or_slug), score)
    (*server.Redis).Do("EXPIRE", CacheKeyForUrlOrSlug(url_or_slug), CACHE_TTL)
}

func (server *Server) GetScoreForUrlOrSlug(url_or_slug string) (string, error) {
    var score string
    var err error
    score, err = server.GetCachedScoreForUrlOrSlug(url_or_slug)
    if err != nil {
        rubyCmd := exec.Command("./get_score.rb", url_or_slug)
        var scoreOut []byte
        scoreOut, err = rubyCmd.Output()
        if err != nil {
            fmt.Println(err)
            return "", err
        }
        lines := strings.Split(string(scoreOut), "\n")
        score = lines[len(lines) - 2]
        server.CacheScoreForUrlOrSlug(score, url_or_slug)
    }

    return score, err
}

func ConnectRedis(redisChannel chan redis.Conn) {
    connection, redisError := redis.Dial("tcp", ":6379")
    if connection != nil {
        redisChannel <- connection
    } else if redisError != nil {
        fmt.Println(redisError)
    } else {
        fmt.Println("Everyting was nil?")
    }
}

func CreateServer(server *Server) {
    m := martini.Classic()
    m.Get("/score(\\.(?P<format>json|html))?", server.GetScore)
    server.Martini = m
    m.Run()
}

func Start(redisChannel chan redis.Conn, retryCount int, server *Server) {
    go ConnectRedis(redisChannel)
    select {
    case redisConnection := <- redisChannel:
        server.Redis = &redisConnection
        fmt.Println("Redis connected")
        defer redisConnection.Close()
        CreateServer(server)
    case <-time.After(time.Second * 1):
        retryCount += 1
        if retryCount < 5 {
            fmt.Println("Retrying Redis connection")
            Start(redisChannel, retryCount, server)
        } else {
            panic("Something is going wrong")
        }
    }

}

func main() {
    redisChannel := make(chan redis.Conn, 1)
    retryCount := 0
    server := Server{}
    Start(redisChannel, retryCount, &server)
}