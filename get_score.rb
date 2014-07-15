#!/usr/bin/env ruby

require 'rubygems'
require 'bundler'
Bundler.require

require 'readme-score'
require 'json'

url_or_slug = ARGV[0]
human_breakdown = ARGV[1].to_s == "true"
document = ReadmeScore.document(url_or_slug)
score = document.score
breakdown = human_breakdown ? score.human_breakdown : score.breakdown
puts({total_score: score.total_score, breakdown: breakdown}.to_json)