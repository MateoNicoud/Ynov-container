VENV := .venv
PLAYBOOK := $(VENV)/bin/ansible-playbook

.PHONY: install deploy syntax

install:
	uv venv $(VENV)
	uv pip install -p $(VENV) -r requirements.txt
	$(VENV)/bin/ansible-galaxy collection install -r requirements.yml -p .vendor

deploy:
	$(PLAYBOOK) site.yml

syntax:
	$(PLAYBOOK) site.yml --syntax-check
