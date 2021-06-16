# sample-embedded-fuse
Sample application to auto mount embedded fs with fuse

## Installation

Requires go 1.16 >

```bash
  GO111MODULE=off go get -u github.com/moisespsena-go/sample-embedded-fuse
  cd $GOPATH/src/github.com/moisespsena-go/sample-embedded-fuse
 ```
 
## Usage

 ```bash
  echo hello > root/hello.txt
  
  # create mount point directory
  mkdir mnt
  # or use `go build -o ./NAME` and exec `./NAME`
  go run main.go ./mnt
```

Show mounted files in another terminal: `ls ./mnt`
