FROM golang:1.24.2-alpine AS build

WORKDIR /src

COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/dagserver ./cmd/dagserver

FROM alpine:3.21

WORKDIR /app

COPY --from=build /out/dagserver /usr/local/bin/dagserver
COPY examples ./examples
COPY web ./web

EXPOSE 8080

CMD ["dagserver", "-addr", "0.0.0.0:8080", "-workflow", "examples/workflow.json", "-clusters", "examples/clusters.json", "-web", "web"]
