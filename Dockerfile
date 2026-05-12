FROM golang:1.22-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/controller ./cmd/controller

FROM alpine:3.20 
#cambiar a distroless 
#gcr.io/distroless/static-debian12 

WORKDIR /app
RUN apk add --no-cache ca-certificates

COPY --from=build /out/controller /usr/local/bin/controller

EXPOSE 5000

CMD ["controller"]

#imagen golang sin bash distroless