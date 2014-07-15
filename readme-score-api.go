package main

import (
    "github.com/go-martini/martini"
    "github.com/martini-contrib/cors"
    "net/http"
    "encoding/json"
    "os/exec"
    "fmt"
    "strings"
    "github.com/garyburd/redigo/redis"
    "github.com/soveran/redisurl"
    "time"
    "os"
)

// Expire caches in an hour
const CACHE_TTL = 60 * 60

type Server struct {
    Redis *redis.Conn
    Martini *martini.ClassicMartini
}

type Score struct {
    TotalScore int `json:"total_score"`
    Breakdown map[string]int `json:"breakdown"`
}

type ScoreResponse struct {
    Score int `json:"score"`
    URL string `json:"url"`
    Breakdown map[string]int `json:"breakdown"`
}

type ErrorResponse struct {
    Error string `json:"error"`
}

func MarshalToJsonBytes(res interface{}) []byte {
    resAsJson, _ := json.Marshal(res)
    return ([]byte(resAsJson))
}

func GetScoreResponseAsJson(score Score, url_or_slug string) []byte {
    res := &ScoreResponse{
        Score:   score.TotalScore,
        Breakdown: score.Breakdown,
        URL: url_or_slug}
    return MarshalToJsonBytes(res)
}

func GetScoreErrorAsJson(url_or_slug string) []byte {
    res := &ErrorResponse{
        Error: "Could not determine score for " + url_or_slug}
    return MarshalToJsonBytes(res)
}

func CacheKeyForUrlOrSlug(url_or_slug string, human_arg string) string{
    return "url_or_slug_v2:" + url_or_slug + ":" + human_arg
}

func (server *Server) GetScore(res http.ResponseWriter, req *http.Request, params martini.Params) {
    res.Header().Set("Content-Type", "application/json")

    query_params := req.URL.Query()

    url_or_slug := ""
    var param_matches []string
    ok := false
    human_breakdown := false
    if param_matches,ok = query_params["url"]; !ok {
        param_matches = query_params["github"]
    }
    url_or_slug = param_matches[0]

    if param_matches, ok = query_params["human_breakdown"]; ok {
        human_breakdown = param_matches[0] == "true"
    }

    score, _ := server.GetScoreForUrlOrSlug(url_or_slug, human_breakdown)

    if score == nil {
        res.Write(GetScoreErrorAsJson(url_or_slug))
    } else {
        res.Write(GetScoreResponseAsJson(*score, url_or_slug))
    }

}

func (server *Server) GetCachedScoreForUrlOrSlug(url_or_slug string, human_arg string) (*Score, error) {
    var score *Score;
    scoreJson, err := redis.String((*server.Redis).Do("GET", CacheKeyForUrlOrSlug(url_or_slug, human_arg)))
    if scoreJson != "" {
        score = &Score{}
        err := json.Unmarshal([]byte(scoreJson), &score)
        if err != nil {
            return nil, err
        }
    }
    return score, err
}

func (server *Server)  CacheScoreForUrlOrSlug(scoreJson string, url_or_slug string, human_arg string) {
    (*server.Redis).Do("SET", CacheKeyForUrlOrSlug(url_or_slug, human_arg), scoreJson)
    (*server.Redis).Do("EXPIRE", CacheKeyForUrlOrSlug(url_or_slug, human_arg), CACHE_TTL)
}

func (server *Server) GetScoreForUrlOrSlug(url_or_slug string, human_breakdown bool) (*Score, error) {
    var score *Score
    var err error
    humanArg := "false"
    if human_breakdown {
        humanArg = "true"
    }
    score, err = server.GetCachedScoreForUrlOrSlug(url_or_slug, humanArg)
    if err != nil {
        rubyCmd := exec.Command("./get_score.rb", url_or_slug, humanArg)
        var scoreOut []byte
        scoreOut, err = rubyCmd.Output()
        if err != nil {
            return nil, err
        }
        lines := strings.Split(string(scoreOut), "\n")
        scoreJson := lines[len(lines) - 2]
        server.CacheScoreForUrlOrSlug(scoreJson, url_or_slug, humanArg)
        score = &Score{}
        err = json.Unmarshal([]byte(scoreJson), &score)
        if err != nil {
            return nil, err
        }
    }

    return score, err
}

func ConnectRedis(redisChannel chan redis.Conn) {
    redisAddress := os.Getenv("REDIS_URL")
    if redisAddress == "" {
        redisAddress = os.Getenv("REDISCLOUD_URL")
        if redisAddress == "" {
            redisAddress = "redis://localhost:6379"
        }
    }

    connection, redisError := redisurl.ConnectToURL(redisAddress)
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
    m.Use(cors.Allow(&cors.Options{
        AllowOrigins:     []string{"*"},
        AllowMethods:     []string{"GET"},
        AllowHeaders:     []string{"Origin"},
    }))
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