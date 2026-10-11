TF := terraform -chdir=infra/terraform
KUBECONFIG_LOCAL := .kube/config

.DEFAULT_GOAL := help
.PHONY: all help check deps tfvars infra cluster up plan status down lint

all: help

help: ## Показать список доступных команд
	@echo "Использование: make <команда>"
	@echo ""
	@echo "Доступные команды:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  make %-10s %s\n", $$1, $$2}'

check: ## Проверить наличие утилит, файла .env и SSH-ключа
	@command -v terraform >/dev/null 2>&1 || (echo "Ошибка: terraform не найден" >&2; exit 1)
	@command -v ansible-playbook >/dev/null 2>&1 || (echo "Ошибка: ansible-playbook не найден" >&2; exit 1)
	@command -v kubectl >/dev/null 2>&1 || (echo "Ошибка: kubectl не найден" >&2; exit 1)
	@command -v ssh >/dev/null 2>&1 || (echo "Ошибка: ssh не найден" >&2; exit 1)
	@if [ ! -f .env ]; then \
		echo "Ошибка: .env не существует. Выполните: cp .env.example .env and edit" >&2; \
		exit 1; \
	fi
	@if [ ! -f $$HOME/.ssh/id_ed25519 ]; then \
		echo "Ошибка: SSH-ключ $$HOME/.ssh/id_ed25519 не найден" >&2; \
		exit 1; \
	fi
	@echo "Проверка пройдена: утилиты, .env и SSH-ключ на месте."

deps: ## Установить коллекции Ansible (infra/ansible/requirements.yml)
	ansible-galaxy collection install -r infra/ansible/requirements.yml

tfvars: ## Сгенерировать infra/terraform/env.auto.tfvars из .env
	@scripts/env-to-tfvars.sh

infra: tfvars ## Развернуть ВМ в Proxmox через Terraform (init + apply)
	$(TF) init -input=false
	$(TF) apply -auto-approve -input=false

cluster: ## Настроить Kubernetes кластер и ArgoCD через Ansible
	ansible-playbook infra/ansible/site.yml

up: check deps infra cluster ## Развернуть стек целиком (Terraform + Ansible)
	@echo ""
	@echo "================================================================="
	@echo "Кластер успешно развернут!"
	@echo "================================================================="
	@echo "Подключение через kubectl:"
	@echo "  export KUBECONFIG=\$$(pwd)/$(KUBECONFIG_LOCAL)"
	@echo "  kubectl get nodes -o wide"
	@echo ""
	@echo "Доступ к веб-интерфейсу ArgoCD:"
	@echo "  kubectl --kubeconfig $(KUBECONFIG_LOCAL) -n argocd port-forward svc/argocd-server 8080:80"
	@echo "  URL: http://localhost:8080 (логин: admin)"
	@echo ""
	@echo "Получение пароля admin для ArgoCD:"
	@echo "  kubectl --kubeconfig $(KUBECONFIG_LOCAL) -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d; echo"
	@echo "================================================================="

plan: tfvars ## Показать план изменений Terraform
	$(TF) plan

status: ## Показать статус нод в Terraform и Kubernetes
	-$(TF) output nodes
	-kubectl --kubeconfig $(KUBECONFIG_LOCAL) get nodes -o wide

down: ## Удалить виртуальные машины в Proxmox (требует подтверждения)
	@echo "ВНИМАНИЕ: Команда безвозвратно удалит все виртуальные машины кластера в Proxmox!"
	@printf "Для подтверждения введите 'yes': "; \
	read ans; \
	if [ "$$ans" = "yes" ]; then \
		$(TF) destroy -auto-approve; \
	else \
		echo "Удаление отменено."; \
	fi

lint: ## Проверить синтаксис Terraform и Ansible
	terraform fmt -check -recursive infra/terraform
	$(TF) validate
	ansible-playbook infra/ansible/site.yml --syntax-check
