package main

import (
    "github.com/go-martini/martini"
    "net/http"
    "encoding/json"
    "os/exec"
    "fmt"
    "strings"
    "github.com/garyburd/redigo/redis"
)

// Expire caches in an hour
const CACHE_TTL = 60 * 60

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

func GetScore(res http.ResponseWriter, req *http.Request, params martini.Params) {
    res.Header().Set("Content-Type", "application/json")

    query_params := req.URL.Query()

    var url_or_slug string
    var param_matches []string
    var ok bool
    if param_matches,ok = query_params["url"]; !ok {
        param_matches = query_params["github"]
    }
    url_or_slug = param_matches[0]

    score, score_error := GetScoreForUrlOrSlug(url_or_slug)

    if score_error != nil {
        res.Write(GetScoreErrorAsJson(url_or_slug))
    } else {
        res.Write(GetScoreResponseAsJson(score, url_or_slug))
    }

}

var redisConnection redis.Conn

func CacheKeyForUrlOrSlug(url_or_slug string) string{
    return "url_or_slug:" + url_or_slug
}

func GetCachedScoreForUrlOrSlug(url_or_slug string) (string, error) {
    score, err := redis.String(redisConnection.Do("GET", CacheKeyForUrlOrSlug(url_or_slug)))
    return score, err
}

func CacheScoreForUrlOrSlug(score string, url_or_slug string) {
    redisConnection.Do("SET", CacheKeyForUrlOrSlug(url_or_slug), score)
    redisConnection.Do("EXPIRE", CacheKeyForUrlOrSlug(url_or_slug), CACHE_TTL)
}

func GetScoreForUrlOrSlug(url_or_slug string) (string, error) {
    var score string
    var err error
    score, err = GetCachedScoreForUrlOrSlug(url_or_slug)
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
        CacheScoreForUrlOrSlug(score, url_or_slug)
    }

    return score, err
}

func main() {
    var redisError error
    redisConnection, redisError = redis.Dial("tcp", ":6379")
    if redisError != nil {
        panic(redisError)
    }
    defer redisConnection.Close()
    m := martini.Classic()
    m.Get("/score(\\.(?P<format>json|html))?", GetScore)
    m.Run()
}