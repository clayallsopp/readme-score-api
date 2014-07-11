#!/usr/bin/env ruby

require 'rubygems'
require 'bundler'
Bundler.require

require 'readme-score'
require 'json'

url_or_slug = ARGV[0]
document = ReadmeScore.document(url_or_slug)
puts document.source_url
puts document.score.total_score