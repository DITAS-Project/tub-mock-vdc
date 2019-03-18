FROM golang:1.12
WORKDIR /opt/dal
COPY . .
RUN CGO_ENABLED=0 go build -a --installsuffix cgo --ldflags="-w -s -X main.Build=$(git rev-parse --short HEAD)" -o mock-vdc
EXPOSE 8080
ENTRYPOINT [ "/opt/dal/mock-vdc" ,"--port","8080"]