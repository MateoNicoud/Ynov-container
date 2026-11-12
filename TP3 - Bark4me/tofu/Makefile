up:
	tofu init
	tofu apply -auto-approve

down:
	tofu destroy -auto-approve

validate:
	tofu init -backend=false
	tofu validate
	tofu fmt -check

.PHONY: up down validate
