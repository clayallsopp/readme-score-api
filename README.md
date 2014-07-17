# readme-score-api [![Readme Score](http://readme-score-api.herokuapp.com/score.svg?url=clayallsopp/readme-score-api)](http://clayallsopp.github.io/readme-score?url=clayallsopp/readme-score-api)

An HTTP API for [readme-score](http://github.com/clayallsopp/readme-score)


## Usage

- The root URL is currently `http://readme-score-api.herokuapp.com`
- The endpoint you want to use is `/score`
- The URL query parameter you want to use is `url`
- `.txt`, `.json`, `.svg` are recognized formats. If something else is used, the response defaults to `.json`
- Scores are currently cached for 1 hour, unless you send a `force` query parameter. Please don't abuse this.

#### Score Data - Text

```sh
$ curl http://readme-score-api.herokuapp.com/score.txt?url=rails/rails -I
Content-Type: text/plain

55
```

#### Score Data - JSON

```sh
$ curl http://readme-score-api.herokuapp.com/score.json?url=rails/rails -I
Content-Type: application/json

{
  "score": 55,
  "url": "rails/rails",
  "breakdown": {
    "cumulative_code_block_length": 0.09001482,
    "has_lists?": 10,
    "low_code_block_penalty": 0,
    "number_of_code_blocks": 15,
    "number_of_gifs": 0,
    "number_of_images": 0,
    "number_of_non_code_sections": 30
  }
}
```

#### Score Data - SVG

```sh
$ curl http://readme-score-api.herokuapp.com/score.svg?url=rails/rails -i
Cache-Control: no-cache
Content-Type: image/svg+xml

<?xml version="1.0" encoding="UTF-8" standalone="no"?>
<svg width="80px" height="18px" viewBox="0 0 80 18" version="1.1" xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" xmlns:sketch="http://www.bohemiancoding.com/sketch/ns">
    <!-- ... --!>
</svg>
```


## Apology

I'm not very awesome at Go, so I'm sorry in advance
