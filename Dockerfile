FROM golang:1.19-alpine as builder

RUN mkdir /app

COPY . /app

WORKDIR /app

RUN CGO_ENABLED=0 go build -o gitOpsController .

FROM alpine:latest

RUN mkdir /app
COPY --from=builder /app/gitOpsController /app

CMD ["/app/gitOpsController"]