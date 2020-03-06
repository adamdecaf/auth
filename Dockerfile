FROM golang:1.14-stretch as builder
WORKDIR /go/src/github.com/moov-io/auth
RUN apt-get update && apt-get install make gcc g++
COPY . .
RUN go mod download
RUN make build

FROM debian:10
RUN apt-get update && apt-get install -y ca-certificates
COPY --from=builder /go/src/github.com/moov-io/auth/bin/auth /bin/auth

VOLUME "/data"
ENV SQLITE_DB_PATH         /data/auth.db
ENV OAUTH2_TOKENS_DB_PATH  /data/oauth2_tokens.db
ENV OAUTH2_CLIENTS_DB_PATH /data/oauth2_clients.db

# RUN adduser -q --gecos '' --disabled-login --shell /bin/false moov
# RUN chown -R moov: /data
# USER moov

EXPOSE 8080
EXPOSE 9090
ENTRYPOINT ["/bin/auth"]
