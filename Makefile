BINARY     := installer/ZealClinic.exe
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
	if exist client\dist rmdir /S /Q client\dist
	xcopy /E /I /Y "$(subst /,\,$(FRONTEND))\dist" client\dist

release: frontend
	go build -trimpath -ldflags "-s -w -H=windowsgui" -o $(BINARY) $(PKG)

installer: release
	$(ISCC) installer/installer.iss

clean:
	-del /Q /F installer\ZealClinic.exe
	-del /Q /F installer\output\ZealClinicSetup-*.exe
	-del /Q /F dist\clinic.db*
	-del /Q /F clinic.db*

help:
	@echo   dev        - build and run as --dev
	@echo   dev-seed   - build and run as --dev --seed-only
	@echo   dev-demo   - build and run as --dev --seed-only --demo
	@echo   run        - build and run as prod
	@echo   build      - debug build to $(BINARY)
	@echo   frontend   - npm build the frontend and copy dist into client/dist
	@echo   release    - prod build to $(BINARY) + embed frontend
	@echo   installer  - release + Inno Setup installer
	@echo   clean      - remove binaries and database
