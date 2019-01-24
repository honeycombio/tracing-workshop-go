# Honeycomb Tracing Workshop Materials

This repository is intended to accompany Honeycomb's [Always Bee Tracing](https://www.eventbrite.com/e/always-bee-tracing-tickets-50756405776) workshop.

It requires Go 1.9+ and has intentionally included the `vendor` directory in hopes of simplifying dependencies. It should require no additional setup or installation beyond cloning this into your `$GOPATH`.

If you'd like to manage / install dependencies yourself, you may want to use [`dep`](https://github.com/golang/dep).

## Running the main application

Run our sample `wall` service with:

```bash
# Will run on port 8080
go run ./wall.go
```

## Interacting with your application

You may either use the web UI to read and write messages:

![index](/images/index.png) | ![new message](/images/message.png)
:-------------------------:|:-------------------------:
View contents of wall | Write new message on wall

Or `curl` the contents of your wall directly:

```bash
# Fetch the contents of your wall
curl localhost:8080
```

```bash
# Write a new message to your wall
curl localhost:8080 -d "message=i'm #tracing with @honeycombio"
```

## Running the analysis service

Over the course of the workshop, you will run a second service, `analysis`, with:

```bash
# Will run on port 8088
go run ./analysis.go
```

But you won't be interacting with it directly; the `wall` service will simply ping `localhost:8088` in hopes of the `analysis` service being alive.
