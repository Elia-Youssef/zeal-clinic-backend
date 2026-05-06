ifeq ($(OS),Windows_NT)
    EXE         := .exe
    LDFLAGS     := -s -w -H=windowsgui
    RM           = del /Q /F $(subst /,\,$(1))
    RMDIR_IF     = if exist $(subst /,\,$(1)) rmdir /S /Q $(subst /,\,$(1))
    COPY_DIST    = xcopy /E /I /Y "$(subst /,\,$(FRONTEND))\dist" client\dist
else
    EXE         :=
    LDFLAGS     := -s -w
    RM           = rm -f $(1)
    RMDIR_IF     = rm -rf $(1)
    COPY_DIST    = cp -r $(FRONTEND)/dist client/dist
endif

BINARY     := installer/ZealClinic$(EXE)
PKG        := ./cmd/server
ISCC       ?= ISCC.exe
FRONTEND   := ../zeal-clinic-frontend

.PHONY: dev dev-seed dev-demo run build frontend release installer clean help

dev: build
	$(BINARY) --dev

dev-seed: build
	$(BINARY) --dev --seed-only

dev-demo: build
	$(BINARY) --dev --seed-only --demo

run: build
	$(BINARY)

build:
	go build -o $(BINARY) $(PKG)

frontend:
	cd $(FRONTEND) && npm run build
	$(call RMDIR_IF,client/dist)
	$(COPY_DIST)

release: frontend
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)

installer: release
	$(ISCC) installer/installer.iss

clean:
	-$(call RM,installer/ZealClinic$(EXE))
	-$(call RM,installer/output/ZealClinicSetup-*.exe)
	-$(call RM,*.pdf)
	-$(call RM,dist/clinic.db*)

help:
	@echo   dev        - build and run as --dev
	@echo   dev-seed   - build and run as --dev --seed-only
	@echo   dev-demo   - build and run as --dev --seed-only --demo
	@echo   run        - build and run as prod
	@echo   build      - debug build to $(BINARY)
	@echo   frontend   - npm build the frontend and copy dist into client/dist
	@echo   release    - prod build to $(BINARY) + embed frontend
	@echo   installer  - release + Inno Setup installer (Windows only)
	@echo   clean      - remove binaries and database
