# How to make

```
go mod init kubackup
go mod tidy
go build .
```

Build with `CGO_ENABLED=0` if preparing for docker image from alpine

# How to run
```
cp env.example env
source ./env
./kubackup
```

# How to decrypt
```
source ./env
./kubackup decrypt < backup.zst.aes > backup.zst
```
