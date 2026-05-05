FROM node:22-alpine AS usage-assets

WORKDIR /app

COPY web/usage-keeper/package.json web/usage-keeper/package-lock.json ./web/usage-keeper/

RUN npm --prefix ./web/usage-keeper ci

COPY web/usage-keeper ./web/usage-keeper

RUN npm --prefix ./web/usage-keeper run build

FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN rm -rf internal/usagekeeper/assets/dist
COPY --from=usage-assets /app/web/usage-keeper/dist ./internal/usagekeeper/assets/dist

ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X 'main.Version=${VERSION}' -X 'main.Commit=${COMMIT}' -X 'main.BuildDate=${BUILD_DATE}'" -o ./CLIProxyAPI ./cmd/server/

FROM alpine:3.23

RUN apk add --no-cache tzdata

RUN mkdir /CLIProxyAPI

COPY --from=builder ./app/CLIProxyAPI /CLIProxyAPI/CLIProxyAPI

COPY config.example.yaml /CLIProxyAPI/config.example.yaml

WORKDIR /CLIProxyAPI

EXPOSE 8317

ENV TZ=Asia/Shanghai

RUN cp /usr/share/zoneinfo/${TZ} /etc/localtime && echo "${TZ}" > /etc/timezone

CMD ["./CLIProxyAPI"]
