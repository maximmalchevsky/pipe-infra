TF := terraform -chdir=infra/terraform
KUBECONFIG_LOCAL := .kube/config
IMAGE ?= $(shell awk '/^image:/{f=1} f && /repository:/{repo=$$2} f && /tag:/{gsub(/"/, "", $$2); tag=$$2; f=0} END{print repo ":" tag}' deploy/apps/notifier/values.yaml)

.DEFAULT_GOAL := help
.PHONY: all help check deps tfvars infra cluster up plan status down lint hosts trust-ca argocd-password apps sealing-key seal load-campaign load-api load-clean watch

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

sealing-key: ## Сгенерировать пару ключей Sealed Secrets (.secrets/sealed-secrets.{key,crt})
	@scripts/gen-sealing-key.sh

seal: ## Зашифровать секреты из .env в SealedSecret манифесты
	@scripts/seal-secrets.sh

infra: tfvars ## Развернуть ВМ в Proxmox через Terraform (init + apply)
	$(TF) init -input=false
	$(TF) apply -auto-approve -input=false

cluster: ## Настроить Kubernetes кластер и ArgoCD через Ansible
	ansible-playbook infra/ansible/site.yml

up: check deps sealing-key infra cluster ## Развернуть стек целиком (Terraform + Ansible)
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

hosts: ## Вывести строку для /etc/hosts (IP MetalLB и домены)
	@DOMAIN=$$(grep -E '^TF_VAR_search_domain=' .env 2>/dev/null | sed -E 's/^TF_VAR_search_domain=["'"'"']?([^"'"'"' ]+).*/\1/' || true); \
	DOMAIN=$${DOMAIN:-hackathon.lan}; \
	IP=$$($(TF) output -raw metallb_ip); \
	echo "$$IP argocd.$$DOMAIN grafana.$$DOMAIN api.$$DOMAIN"; \
	echo "Для добавления в /etc/hosts выполните: make hosts | sudo tee -a /etc/hosts" >&2

trust-ca: ## Экспортировать CA кластера и показать команду для macOS Keychain
	kubectl --kubeconfig $(KUBECONFIG_LOCAL) -n cert-manager get secret hackathon-root-ca -o jsonpath='{.data.ca\.crt}' | base64 -d > .kube/hackathon-root-ca.crt
	@echo "Для добавления сертификата в доверенные macOS выполните:"
	@echo "  sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain .kube/hackathon-root-ca.crt"

argocd-password: ## Получить первоначальный пароль admin для ArgoCD
	@kubectl --kubeconfig $(KUBECONFIG_LOCAL) -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d; echo

apps: ## Показать список Application в ArgoCD
	kubectl --kubeconfig $(KUBECONFIG_LOCAL) -n argocd get applications

load-campaign: ## Запустить генерацию кампании рассылки через Kubernetes Job (N=50000)
	@N_VAL=$${N:-50000}; \
	EPOCH=$$(date +%s); \
	kubectl --kubeconfig $(KUBECONFIG_LOCAL) -n notifier create job loadgen-campaign-$$EPOCH --image=$(IMAGE) -- /usr/local/bin/loadgen campaign -url http://notifier-api-stable.notifier.svc/api/v1/campaigns -recipients $$N_VAL

load-api: ## Запустить нагрузочное HTTP-тестирование API через Kubernetes Job (RPS=300, DURATION=2m)
	@RPS_VAL=$${RPS:-300}; \
	DUR_VAL=$${DURATION:-2m}; \
	EPOCH=$$(date +%s); \
	kubectl --kubeconfig $(KUBECONFIG_LOCAL) -n notifier create job loadgen-api-$$EPOCH --image=$(IMAGE) -- /usr/local/bin/loadgen http -url http://notifier-api-stable.notifier.svc/api/v1/campaigns -rps $$RPS_VAL -duration $$DUR_VAL

load-clean: ## Удалить завершенные Kubernetes Jobs генератора нагрузки (loadgen-*)
	@for j in $$(kubectl --kubeconfig $(KUBECONFIG_LOCAL) -n notifier get jobs -o jsonpath='{range .items[?(@.status.succeeded > 0)]}{.metadata.name}{" "}{end}' 2>/dev/null); do \
		case "$$j" in loadgen-*) kubectl --kubeconfig $(KUBECONFIG_LOCAL) -n notifier delete job "$$j" ;; esac; \
	done

watch: ## Отслеживать в реальном времени статус KEDA ScaledObjects, поды и размер очереди
	@while true; do \
		clear; \
		echo "=== ScaledObjects (KEDA) ==="; \
		kubectl --kubeconfig $(KUBECONFIG_LOCAL) -n notifier get scaledobjects 2>/dev/null || true; \
		echo ""; \
		echo "=== Pods (API & Worker) ==="; \
		kubectl --kubeconfig $(KUBECONFIG_LOCAL) -n notifier get pods -l 'app.kubernetes.io/name in (notifier-api, notifier-worker)' 2>/dev/null || true; \
		echo ""; \
		echo "=== Pending Queue (notifier-db-1) ==="; \
		kubectl --kubeconfig $(KUBECONFIG_LOCAL) -n notifier exec -i notifier-db-1 -c postgres -- psql -U notifier -d notifier -t -A -c "SELECT count(*) FROM messages WHERE status='pending';" 2>/dev/null || echo "N/A"; \
		sleep 2; \
	done
