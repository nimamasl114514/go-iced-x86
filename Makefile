RUST_DIR := rust-iced
UNAME_S  := $(shell uname -s 2>/dev/null || echo Windows)

ifeq ($(OS),Windows_NT)
  LIB := iced_go_ffi.dll
else ifeq ($(UNAME_S),Darwin)
  LIB := libiced_go_ffi.dylib
else
  LIB := libiced_go_ffi.so
endif

.PHONY: all rust go test clean

all: rust go

rust:
	cd $(RUST_DIR) && cargo build --release
	cp $(RUST_DIR)/target/release/$(LIB) ./$(LIB)

go:
	CGO_ENABLED=0 go mod tidy
	CGO_ENABLED=0 go build ./...

test: all
	CGO_ENABLED=0 go test -v .

clean:
	cd $(RUST_DIR) && cargo clean
	rm -f $(LIB)

