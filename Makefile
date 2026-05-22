# Build version — single source of truth, shared with build/local/installer.iss.
VERSION := $(strip $(file < VERSION))

ifeq ($(OS),Windows_NT)
    EXE         := .exe
    LDFLAGS     := -s -w -H=windowsgui -X clinic-api/internal/buildmode.Version=$(VERSION)
    RM           = del /Q /F $(subst /,\,$(1))
    RM_R         = del /S /Q /F $(1)
    RMDIR_IF     = if exist $(subst /,\,$(1)) rmdir /S /Q $(subst /,\,$(1))
    MKDIR_P      = if not exist $(subst /,\,$(1)) mkdir $(subst /,\,$(1))
    COPY_DIST    = xcopy /E /I /Y "$(subst /,\,$(FRONTEND))\dist" client\dist
    CLOUD_ENV   := set CGO_ENABLED=0&& set GOOS=linux&& set GOARCH=amd64&&
else
    EXE         :=
    LDFLAGS     := -s -w -X clinic-api/internal/buildmode.Version=$(VERSION)
    RM           = rm -f $(1)
    RM_R         = find . -name '$(1)' -type f -delete
    RMDIR_IF     = rm -rf $(1)
    MKDIR_P      = mkdir -p $(1)
    COPY_DIST    = cp -r $(FRONTEND)/dist client/dist
    CLOUD_ENV   := CGO_ENABLED=0 GOOS=linux GOARCH=amd64
endif

# Cloud ldflags mirror LDFLAGS but never set -H=windowsgui (cloud is a server).
CLOUD_LDFLAGS := -s -w -X clinic-api/internal/buildmode.Version=$(VERSION)
# Debug builds keep symbols but stamp a -dev suffix on the version.
DEV_LDFLAGS   := -X clinic-api/internal/buildmode.Version=$(VERSION)-dev

# Debug binaries live in tmp/ (gitignored) alongside the dev clinic.db.
# Release artifacts live under build/<target>/output/.
DEV_BINARY        := tmp/ZealClinic_dev$(EXE)
DEV_CLOUD_BINARY  := tmp/ZealClinicCloud_dev$(EXE)
LOCAL_BINARY      := build/local/output/ZealClinic$(EXE)
CLOUD_BINARY      := build/cloud/output/ZealClinicCloud-$(VERSION)-linux-amd64
PKG               := ./cmd/server
ISCC              ?= ISCC.exe
FRONTEND          := ../zeal-clinic-frontend

.PHONY: dev dev-seed dev-demo dev-cloud build build-cloud frontend release release-cloud installer deploy clean help

dev: build
	$(DEV_BINARY) --dev

dev-seed: build
	$(DEV_BINARY) --dev --seed-only

dev-demo: build
	$(DEV_BINARY) --dev --seed-only --demo

dev-cloud: build-cloud
	$(DEV_CLOUD_BINARY) --dev

build:
	$(call MKDIR_P,tmp)
	go build -ldflags "$(DEV_LDFLAGS)" -o $(DEV_BINARY) $(PKG)

# Native cloud-tagged debug build so the cloud code path is runnable locally.
build-cloud:
	$(call MKDIR_P,tmp)
	go build -tags cloud -ldflags "$(DEV_LDFLAGS)" -o $(DEV_CLOUD_BINARY) $(PKG)

frontend:
	cd $(FRONTEND) && npm run build
	$(call RMDIR_IF,client/dist)
	$(COPY_DIST)

# release / release-cloud reuse the existing client/dist — run `make frontend`
# (or `make release-all`) first so one frontend build serves both platforms.
release:
	$(call MKDIR_P,build/local/output)
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(LOCAL_BINARY) $(PKG)

release-cloud:
	$(call MKDIR_P,build/cloud/output)
	$(CLOUD_ENV) go build -tags cloud -trimpath -ldflags "$(CLOUD_LDFLAGS)" -o $(CLOUD_BINARY) $(PKG)

installer: release
	$(ISCC) build/local/installer.iss

# Full deploy: build the frontend once, then the cloud release and the
# Windows installer (which also produces the local release) against it.
deploy: frontend release-cloud installer

clean:
	-$(call RMDIR_IF,tmp)
	-$(call RMDIR_IF,build/local/output)
	-$(call RMDIR_IF,build/cloud/output)
	-$(call RM_R,*.pdf)

help:
	@echo "dev            - build and run as --dev (binary in tmp/)"
	@echo "dev-seed       - build and run as --dev --seed-only"
	@echo "dev-demo       - build and run as --dev --seed-only --demo"
	@echo "dev-cloud      - cloud build and run as --dev (binary in tmp/)"
	@echo "build          - debug build to $(DEV_BINARY)"
	@echo "build-cloud    - cloud debug build to $(DEV_CLOUD_BINARY)"
	@echo "frontend       - npm build the frontend and copy dist into client/dist"
	@echo "release        - prod build to $(LOCAL_BINARY) (needs client/dist)"
	@echo "release-cloud  - cloud prod build to $(CLOUD_BINARY) (needs client/dist)"
	@echo "installer      - release + Inno Setup installer (Windows only)"
	@echo "deploy         - frontend + release-cloud + installer"
	@echo "clean          - remove build outputs and tmp/"
