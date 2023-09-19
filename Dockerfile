FROM golang:1.21-alpine3.17 AS builder-base

ENV build_workdir /go/src
ENV GOPROXY https://goproxy.cn
WORKDIR $build_workdir

#构建运行环境
COPY ./go.mod $build_workdir/
COPY ./go.sum $build_workdir/
RUN go mod download

#构建可执行文件
COPY ./ $build_workdir/
RUN CGO_ENABLED=0 GOARCH=amd64 GOOS=linux \
    && go build -ldflags '-w -s' -o $build_workdir/main $build_workdir/

#第二阶段压缩镜像体积
FROM 451008604/alpine:3.17
WORKDIR /app

#拷贝运行所需文件
COPY --from=builder-base /go/src/main /app/

ENTRYPOINT ["./main"]
