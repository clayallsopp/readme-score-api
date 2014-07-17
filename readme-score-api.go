package main

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/garyburd/redigo/redis"
	"github.com/go-martini/martini"
	"github.com/martini-contrib/cors"
	"github.com/soveran/redisurl"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/template"
	"time"
)

// Expire caches in an hour
const CACHE_TTL = 60 * 60

type Server struct {
	Redis   *redis.Conn
	Martini *martini.ClassicMartini
}

type Score struct {
	TotalScore     float32              `json:"total_score"`
	Breakdown      map[string]float32   `json:"breakdown"`
	HumanBreakdown map[string][]float32 `json:"human_breakdown"`
}

type ScoreResponse struct {
	Score     float32            `json:"score"`
	URL       string             `json:"url"`
	Breakdown map[string]float32 `json:"breakdown"`
}

type HumanScoreResponse struct {
	Score     float32              `json:"score"`
	URL       string               `json:"url"`
	Breakdown map[string][]float32 `json:"breakdown"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func MarshalToJsonBytes(res interface{}) []byte {
	resAsJson, _ := json.Marshal(res)
	return ([]byte(resAsJson))
}

func GetScoreResponseAsJson(score Score, url_or_slug string) []byte {
	var res interface{}

	if score.Breakdown != nil {
		res = &ScoreResponse{
			Score:     score.TotalScore,
			Breakdown: score.Breakdown,
			URL:       url_or_slug}
	} else {
		res = &HumanScoreResponse{
			Score:     score.TotalScore,
			Breakdown: score.HumanBreakdown,
			URL:       url_or_slug}
	}

	return MarshalToJsonBytes(res)
}

func (score Score) AsColor() string {
	if score.TotalScore < 25 {
		return "#E74C3C"
	}
	if score.TotalScore < 80 {
		return "#F39C12"
	}
	return "#2ECC71"
}

var score_template_string = ""
var score_template = template.New("score template")

func GetScoreResponseAsSVG(score Score, url_or_slug string) []byte {
	var doc bytes.Buffer
	var err error

	if score_template_string == "" {
		var score_template_bytes []byte
		if score_template_bytes, err = ioutil.ReadFile("./templates/score.svg"); err == nil {
			score_template_string = string(score_template_bytes)
		}
		score_template, err = score_template.Parse(score_template_string)
	}

	if err == nil {
		err = score_template.Execute(&doc, score)
	}
	HandleError(err)

	return doc.Bytes()
}

var error_template = ""

func GetScoreErrorAsSVG() []byte {
	if error_template == "" {
		if error_template_bytes, err := ioutil.ReadFile("./templates/error.svg"); err != nil {
			HandleError(err)
		} else {
			error_template = string(error_template_bytes)
		}
	}

	return ([]byte(error_template))
}

func GetScoreErrorAsJson(url_or_slug string) []byte {
	res := &ErrorResponse{
		Error: "Could not determine score for " + url_or_slug}
	return MarshalToJsonBytes(res)
}

func CacheKeyForUrlOrSlug(url_or_slug string, human_arg string) string {
	return "url_or_slug_v3:" + url_or_slug + ":" + human_arg
}

func WriteSVGWithETag(res http.ResponseWriter, body []byte) {
	hash := md5.New()
	io.WriteString(hash, string(body))
	etag := fmt.Sprintf("\"%x\"", hash.Sum(nil))
	res.Header().Set("ETag", etag)
	res.Write(body)
}

func (server *Server) GetScore(res http.ResponseWriter, req *http.Request, params martini.Params) {
	query_params := req.URL.Query()
	url_or_slug := ""
	ok := false
	human_breakdown := false
	hit_cache := true
	format := params["format"]
	if format == "svg" {
		res.Header().Set("Content-Type", "image/svg+xml")
		res.Header().Set("Cache-Control", "no-cache, private")
	} else if format == "txt" {
		res.Header().Set("Content-Type", "text/plain")
	} else {
		res.Header().Set("Content-Type", "application/json")
	}
	var param_matches []string
	var score *Score
	var err error

	if param_matches, ok = query_params["url"]; !ok {
		param_matches = query_params["github"]
	}
	if len(param_matches) == 0 {
		err = errors.New("No value for :url or :github query parameter")
	} else {
		url_or_slug = param_matches[0]

		if param_matches, ok = query_params["human_breakdown"]; ok {
			human_breakdown = param_matches[0] == "true"
		}

		if param_matches, ok = query_params["force"]; ok {
			hit_cache = false
		}

		score, err = server.GetScoreForUrlOrSlug(url_or_slug, human_breakdown, hit_cache)
	}
	HandleError(err)

	if score == nil {
		if format == "svg" {
			WriteSVGWithETag(res, GetScoreErrorAsSVG())
		} else if format == "txt" {
			res.Write([]byte("error"))
		} else {
			res.Write(GetScoreErrorAsJson(url_or_slug))
		}
	} else {
		if format == "svg" {
			WriteSVGWithETag(res, GetScoreResponseAsSVG(*score, url_or_slug))
		} else if format == "txt" {
			res.Write([]byte(strconv.Itoa(int(score.TotalScore))))
		} else {
			res.Write(GetScoreResponseAsJson(*score, url_or_slug))
		}
	}

}

func (server *Server) GetCachedScoreForUrlOrSlug(url_or_slug string, human_arg string) (*Score, error) {
	var score *Score
	scoreJson, err := redis.String((*server.Redis).Do("GET", CacheKeyForUrlOrSlug(url_or_slug, human_arg)))
	if scoreJson != "" {
		score = &Score{}
		if err = json.Unmarshal([]byte(scoreJson), &score); err != nil {
			score = nil
		}
	}

	return score, err
}

func (server *Server) CacheScoreForUrlOrSlug(scoreJson string, url_or_slug string, human_arg string) {
	(*server.Redis).Do("SET", CacheKeyForUrlOrSlug(url_or_slug, human_arg), scoreJson)
	(*server.Redis).Do("EXPIRE", CacheKeyForUrlOrSlug(url_or_slug, human_arg), CACHE_TTL)
}

func (server *Server) GetScoreForUrlOrSlug(url_or_slug string, human_breakdown bool, hit_cache bool) (*Score, error) {
	var score *Score
	var err error
	humanArg := "false"
	if human_breakdown {
		humanArg = "true"
	}
	if score, err = server.GetCachedScoreForUrlOrSlug(url_or_slug, humanArg); err != nil || !hit_cache {
		rubyCmd := exec.Command("./get_score.rb", url_or_slug, humanArg)
		var scoreOut []byte
		if scoreOut, err = rubyCmd.Output(); err == nil {
			lines := strings.Split(string(scoreOut), "\n")
			scoreJson := lines[len(lines)-2]
			server.CacheScoreForUrlOrSlug(scoreJson, url_or_slug, humanArg)
			score = &Score{}
			if err = json.Unmarshal([]byte(scoreJson), &score); err != nil {
				score = nil
			}
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
		ExposeHeaders:    []string{"Content-Type, Cache-Control, Expires, Etag, Last-Modified"},
		AllowCredentials: true,
	}))
	m.Get("/score(\\.(?P<format>json|html|svg|txt))?", server.GetScore)
	server.Martini = m
	m.Run()
}

func Start(redisChannel chan redis.Conn, retryCount int, server *Server) {
	go ConnectRedis(redisChannel)
	select {
	case redisConnection := <-redisChannel:
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
