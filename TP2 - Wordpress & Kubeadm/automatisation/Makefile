VENV := .venv
PLAYBOOK := $(VENV)/bin/ansible-playbook
VAULT := $(if $(ANSIBLE_VAULT_PASSWORD_FILE),,--ask-vault-pass)

.PHONY: install deploy check snapshot

install:
	uv venv $(VENV)
	uv pip install -p $(VENV) -r requirements.txt
	$(VENV)/bin/ansible-galaxy collection install -r requirements.yml -p .vendor

deploy:
	$(PLAYBOOK) site.yml $(VAULT)

check:
	$(PLAYBOOK) check.yml

snapshot:
	$(PLAYBOOK) 11-etcd-snapshot.yml
