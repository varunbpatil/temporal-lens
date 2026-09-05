FROM node:24-alpine AS ui
WORKDIR /app/ui
COPY ui/package.json ui/package-lock.json ./
RUN npm ci
COPY ui/ .
RUN npm run build

FROM golang:1.27.1-alpine AS build
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui /app/ui/dist ./ui/dist
RUN go build -tags=ui \
    -ldflags "-X github.com/varunbpatil/temporal-lens/version.Version=${VERSION} -X github.com/varunbpatil/temporal-lens/version.GitCommit=${COMMIT} -X github.com/varunbpatil/temporal-lens/version.BuildTime=${BUILD_TIME}" \
    -o bin/temporal-lens ./cmd/workflows

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /app/bin/temporal-lens /usr/local/bin/temporal-lens
ENTRYPOINT ["temporal-lens"]
