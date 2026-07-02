# ==== Config ====
BINDIR      := bin
KUBOS_BIN   := $(BINDIR)/kubos
TEST_BIN    := $(BINDIR)/test
GOFLAGS     ?=
LDFLAGS     ?=

# System layout (root-level)
SYS_CONFIG  := /etc/kubos
SYS_PERSIST := /var/lib/kubos
SYS_LOG     := /var/log/kubos

# User layout (per-user)
HOME_DIR    := $(HOME)
USR_CONFIG  := $(HOME_DIR)/.config/kubos
USR_PERSIST := $(HOME_DIR)/.local/share/kubos
USR_LOG     := $(USR_DIR)/.local/state/kubos

.PHONY: all build build-kubos build-test \
        setup-system setup-user \
        clean test

# ==== Build ====
all: build

build: build-kubos build-test

build-kubos:
	@mkdir -p $(BINDIR)
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(KUBOS_BIN) ./cmd/kubos

build-test:
	@mkdir -p $(BINDIR)
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(TEST_BIN) ./cmd/test

# ==== Layout setup ====
# Perlu sudo/root karena nulis ke /etc, /var
setup-system:
	@echo "Setting up system layout..."
	install -d -m 0755 $(SYS_CONFIG)
	install -d -m 0755 $(SYS_PERSIST)
	install -d -m 0755 $(SYS_LOG)
	@echo "System layout ready:"
	@echo "  config : $(SYS_CONFIG)"
	@echo "  persist: $(SYS_PERSIST)"
	@echo "  log    : $(SYS_LOG)"

setup-user:
	@echo "Setting up user layout for $(HOME_DIR)..."
	mkdir -p $(USR_CONFIG)
	mkdir -p $(USR_PERSIST)
	mkdir -p $(USR_LOG)
	@echo "User layout ready:"
	@echo "  config : $(USR_CONFIG)"
	@echo "  persist: $(USR_PERSIST)"
	@echo "  log    : $(USR_LOG)"

# Jalankan keduanya kalau mau full setup (system biasanya butuh sudo)
setup: setup-user
	@echo "Run 'sudo make setup-system' if you need the system-wide layout too."

# ==== Misc ====
test:
	go test ./...

clean:
	rm -rf $(BINDIR)