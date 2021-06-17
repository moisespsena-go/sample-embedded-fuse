# sample-embedded-fuse
Sample application to auto mount embedded fs with fuse

## Installation

Requires go 1.16 >

```bash
  GO111MODULE=off go get -u github.com/moisespsena-go/sample-embedded-fuse
  cd $GOPATH/src/github.com/moisespsena-go/sample-embedded-fuse
 ```
 
## Usage

- Master application script: `root/main.sh`. 
- Put your service source code into `root/service/`.
- Put your static or assets int `root/static/`.

Run:
 ```bash  
  go run .
```

or

```bash  
  go build . -o my_app
  ./my_app
```

Show mounted files in another terminal: `ls ./mnt`
