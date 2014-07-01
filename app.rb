require 'rubygems'
require 'bundler'
Bundler.require

require 'json'

set :root, File.dirname(__FILE__)


get '/score.?:format?' do
  content_type :json

  url_or_slug = params[:url] || params[:github]

  begin
    score = ReadmeScore.for(url_or_slug).total_score
  rescue
    return [404, {error: "Could not determine score for #{url_or_slug}"}.to_json]
  end

  {score: score, url: url_or_slug}.to_json
end