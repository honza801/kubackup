# how to make

```
go mod init kubackup
go mod tidy
go build .
```

# how to run
```
export AWS_ACCESS_KEY_ID=tester
export AWS_SECRET_ACCESS_KEY=testerpass
#export S3_ENDPOINT=https://my.minio.test:9000
#export S3_BUCKET=kubackup
./kubackup
```
