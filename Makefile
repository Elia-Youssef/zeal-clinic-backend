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

# Dev binary lives in tmp/ (gitignored) alongside the dev clinic.db.
# Release artifacts live under build/<target>/output/.
DEV_BINARY    := tmp/ZealClinic_dev$(EXE)
LOCAL_BINARY  := build/local/output/ZealClinic$(EXE)
CLOUD_BINARY  := build/cloud/output/ZealClinicCloud-$(VERSION)-linux-amd64
PKG           := ./cmd/server
ISCC          ?= ISCC.exe
FRONTEND      := ../zeal-clinic-frontend

.PHONY: dev dev-seed dev-demo run build frontend release installer build-cloud release-cloud clean help

dev: build
	$(DEV_BINARY) --dev

dev-seed: build
	$(DEV_BINARY) --dev --seed-only

dev-demo: build
	$(DEV_BINARY) --dev --seed-only --demo

run: build
	$(DEV_BINARY)

build:
	$(call MKDIR_P,tmp)
	go build -o $(DEV_BINARY) $(PKG)

frontend:
	cd $(FRONTEND) && npm run build
	$(call RMDIR_IF,client/dist)
	$(COPY_DIST)

release: frontend
	$(call MKDIR_P,build/local/output)
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(LOCAL_BINARY) $(PKG)

installer: release
	$(ISCC) build/local/installer.iss

build-cloud:
	$(call MKDIR_P,build/cloud/output)
	$(CLOUD_ENV) go build -tags cloud -o $(CLOUD_BINARY) $(PKG)

release-cloud: frontend
	$(call MKDIR_P,build/cloud/output)
	$(CLOUD_ENV) go build -tags cloud -trimpath -ldflags "-s -w -X clinic-api/internal/buildmode.Version=$(VERSION)" -o $(CLOUD_BINARY) $(PKG)

clean:
	-$(call RMDIR_IF,tmp)
	-$(call RMDIR_IF,build/local/output)
	-$(call RMDIR_IF,build/cloud/output)
	-$(call RM_R,*.pdf)

help:
	@echo "dev            - build and run as --dev (binary in tmp/)"
	@echo "dev-seed       - build and run as --dev --seed-only"
	@echo "dev-demo       - build and run as --dev --seed-only --demo"
	@echo "run            - build and run as prod (dev binary)"
	@echo "build          - debug build to $(DEV_BINARY)"
	@echo "frontend       - npm build the frontend and copy dist into client/dist"
	@echo "release        - prod build to $(LOCAL_BINARY) + embed frontend"
	@echo "installer      - release + Inno Setup installer (Windows only)"
	@echo "build-cloud    - cloud build to $(CLOUD_BINARY) (no tray, no browser)"
	@echo "release-cloud  - cloud prod build + embed frontend (run from WSL)"
	@echo "clean          - remove build outputs and tmp/"
