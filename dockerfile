FROM golang:alpine
WORKDIR /build
COPY go.mod .
COPY go.sum .
COPY .env .
COPY api-slack-bot.go .
RUN go mod download
RUN go mod vendor
RUN go mod tidy
RUN go build -o api-slack-bot api-slack-bot.go
CMD ["./api-slack-bot"]
