BIN := bin/astral-spy
SRC := main.go internal/sus/sus.go internal/sus/metrics.go

.PHONY: all clean
all: $(BIN)

$(BIN): $(SRC) go.mod go.sum | bin
	go build -o $@ .

bin:
	mkdir -p bin

clean:
	rm -rf bin
