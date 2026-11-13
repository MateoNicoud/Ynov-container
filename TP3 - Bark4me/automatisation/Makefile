VENV := .venv
PLAYBOOK := $(VENV)/bin/ansible-playbook

.PHONY: install deploy check check-platform check-app syntax scenarios scenario-drain scenario-node-stop scenario-longhorn scenario-postgres

install:
	uv venv $(VENV)
	uv pip install -p $(VENV) -r requirements.txt
	$(VENV)/bin/ansible-galaxy collection install -r requirements.yml -p .vendor

deploy:
	$(PLAYBOOK) site.yml

# check = plateforme + application. check-platform marche sans l'application déployée.
check:
	$(PLAYBOOK) check.yml

check-platform:
	$(PLAYBOOK) check-platform.yml

check-app:
	$(PLAYBOOK) check-app.yml

syntax:
	$(PLAYBOOK) site.yml --syntax-check
	$(PLAYBOOK) check.yml --syntax-check

# Scénarios de panne : chacun sonde le site pendant la panne et vérifie le retour à la normale.
scenario-drain:
	./scenarios/drain.sh

scenario-node-stop:
	./scenarios/node-stop.sh

scenario-longhorn:
	./scenarios/longhorn-rebuild.sh

scenario-postgres:
	./scenarios/postgres-pod.sh

scenarios: scenario-drain scenario-node-stop scenario-longhorn scenario-postgres
