# FROM golang:1-alpine as builder
# ENV GOOS=linux GOARCH=amd64 CGO_ENAGLED=0
# RUN apk add g++
# WORKDIR /build
# COPY go.mod go.sum ./
# RUN go mod download
# COPY . .
# RUN go build -a -ldflags "-extldflags '-static -O3' -s -w" -o metrico main.go
# # RUN go build -a -ldflags "-extldflags '-static -O3' -s -w" -o metrico main.go

# FROM gcr.io/distroless/base-debian11
# COPY --from=builder /build/metrico /metrico
# ENTRYPOINT ["/metrico"]

FROM golang:1-bullseye as builder
ENV GOOS=linux GOARCH=amd64 CGO_ENAGLED=0
# RUN apk add g++
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -a -ldflags "-s -w" -o metrico main.go
# RUN go build -a -ldflags "-extldflags '-static -O3' -s -w" -o metrico main.go

FROM debian:bullseye
COPY --from=builder /build/metrico /metrico
ENTRYPOINT ["/metrico"]
