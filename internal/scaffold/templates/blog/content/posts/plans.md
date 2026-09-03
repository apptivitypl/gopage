title: writing without a database
date: 2026-02-02
summary: files are a fine content store when the content is written by hand

A blog of a few dozen posts does not need a database. Files are versioned, diffable, and readable
without a running process.

The list page reads every file once per request and the response cache keeps the result warm.
