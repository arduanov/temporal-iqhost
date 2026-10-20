FROM node:22-alpine AS frontend
WORKDIR /web
COPY web/admin/package*.json ./
RUN npm ci
COPY web/admin/ ./
RUN npm run build
FROM golang:1.27.2-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /bin/iqhost-demo/ ./cmd/...
FROM alpine:3.21.3
RUN apk add --no-cache ca-certificates && adduser -D iqhost && mkdir /var/lib/iqhost-simulator && chown iqhost:iqhost /var/lib/iqhost-simulator
USER iqhost
COPY --from=build /bin/iqhost-demo/ /usr/local/bin/
COPY --from=frontend /web/dist/ /opt/iqhost/admin/
ENTRYPOINT ["iqhost"]
