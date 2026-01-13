FROM golang AS build
WORKDIR /gomento
COPY . .
RUN CGO_ENABLED=0 go build -o /go/bin/gomento ./cmd/gomento

FROM alpine
RUN apk --no-cache add ca-certificates
COPY --from=build /go/bin/gomento /bin/gomento
ENTRYPOINT [ "/bin/gomento" ]