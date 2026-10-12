TF := terraform -chdir=infra/terraform
KUBECONFIG_LOCAL := .kube/config
# Доп. аргументы ansible-playbook, напр. ANSIBLE_ARGS="-e cni=cilium"
ANSIBLE_ARGS ?=
# Доп. аргументы terraform, напр. TF_ARGS="-var control_count=3"
TF_ARGS ?=
IMAGE ?= $(shell awk '/^image:/{f=1} f && /repository:/{repo=$$2} f && /tag:/{gsub(/"/, "", $$2); tag=$$2; f=0} END{print repo ":" tag}' deploy/apps/notifier/values.yaml)
DOMAIN ?= $(shell grep -E '^TF_VAR_search_domain=' .env 2>/dev/null | sed -E 's/^TF_VAR_search_domain=["'"'"']?([^"'"'"' ]+).*/\1/' || true)
ifeq ($(DOMAIN),)
DOMAIN := hackathon.lan
endif

.DEFAULT_GOAL := help
.PHONY: all help check deps tfvars infra cluster up wait plan status down lint hosts trust-ca argocd-password apps sealing-key seal load-campaign load-api load-clean watch demo-canary demo-rollback demo-fix rollout-status

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
	$(TF) apply -auto-approve -input=false $(TF_ARGS)

cluster: ## Настроить Kubernetes кластер и ArgoCD через Ansible
	ansible-playbook infra/ansible/site.yml $(ANSIBLE_ARGS)

up: check deps sealing-key infra cluster wait ## Развернуть стек целиком (Terraform + Ansible)
	@echo ""
	@echo "================================================================="
	@echo "Кластер успешно развернут!"
	@echo "================================================================="
	@echo "Подключение через kubectl:"
	@echo "  export KUBECONFIG=\$$(pwd)/$(KUBECONFIG_LOCAL)"
	@echo "  kubectl get nodes -o wide"
	@echo ""
	@echo "Веб-интерфейсы (Ingress):"
	@echo "  ArgoCD:  https://argocd.$(DOMAIN) (логин: admin)"
	@echo "  Grafana: https://grafana.$(DOMAIN)"
	@echo "  API:     https://api.$(DOMAIN)"
	@echo ""
	@echo "Настройка доступа (выполнить один раз):"
	@echo "  make hosts | sudo tee -a /etc/hosts"
	@echo "  make trust-ca"
	@echo ""
	@echo "Получение пароля admin для ArgoCD:"
	@echo "  kubectl --kubeconfig $(KUBECONFIG_LOCAL) -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d; echo"
	@echo "================================================================="

plan: tfvars ## Показать план изменений Terraform
	$(TF) plan $(TF_ARGS)

status: ## Показать статус нод в Terraform и Kubernetes
	-$(TF) output nodes
	-kubectl --kubeconfig $(KUBECONFIG_LOCAL) get nodes -o wide

down: ## Удалить виртуальные машины в Proxmox (требует подтверждения)
	@echo "ВНИМАНИЕ: Команда безвозвратно удалит все виртуальные машины кластера в Proxmox!"
	@printf "Для подтверждения введите 'yes': "; \
	read ans; \
	if [ "$$ans" = "yes" ]; then \
		$(TF) destroy -auto-approve $(TF_ARGS); \
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

wait: ## Дождаться, пока все Application в ArgoCD станут Synced/Healthy (таймаут WAIT_TIMEOUT, по умолчанию 40 мин)
	@WAIT_TIMEOUT=$${WAIT_TIMEOUT:-2400}; \
	ELAPSED=0; \
	echo "Ожидание готовности ArgoCD Applications (таймаут $${WAIT_TIMEOUT}с)..."; \
	while [ $$ELAPSED -lt $$WAIT_TIMEOUT ]; do \
		RAW=$$(kubectl --kubeconfig $(KUBECONFIG_LOCAL) -n argocd get applications -o jsonpath='{range .items[*]}{.metadata.name}{" "}{.status.sync.status}{" "}{.status.health.status}{"\n"}{end}' 2>/dev/null || true); \
		SUMMARY=$$(printf "%s\n" "$$RAW" | awk 'NF>=1 { total++; if ($$2 == "Synced" && $$3 == "Healthy") ready++; else not_ready = not_ready " " $$1 "(" $$2 "/" $$3 ")"; } END { printf "%d %d%s", total+0, ready+0, not_ready }'); \
		TOTAL=$$(echo "$$SUMMARY" | awk '{print $$1}'); \
		READY=$$(echo "$$SUMMARY" | awk '{print $$2}'); \
		NOT_READY=$$(echo "$$SUMMARY" | cut -d" " -f3-); \
		if [ "$$TOTAL" -gt 0 ] && [ "$$READY" -eq "$$TOTAL" ]; then \
			echo "healthy $$READY/$$TOTAL — все приложения Synced/Healthy!"; \
			exit 0; \
		fi; \
		echo "healthy $$READY/$$TOTAL (не готовы: $${NOT_READY:-ожидание приложений})"; \
		sleep 15; \
		ELAPSED=$$((ELAPSED + 15)); \
	done; \
	echo "Таймаут ($${WAIT_TIMEOUT}с) ожидания ArgoCD Applications! Не готовы:$$NOT_READY" >&2; \
	exit 1

apps: ## Показать список Application в ArgoCD
	kubectl --kubeconfig $(KUBECONFIG_LOCAL) -n argocd get applications

load-campaign: ## Запустить генерацию кампании рассылки через Kubernetes Job (N=50000)
	@N_VAL=$${N:-50000}; \
	EPOCH=$$(date +%s); \
	scripts/loadgen-job.sh loadgen-campaign-$$EPOCH $(IMAGE) campaign -url https://ingress-nginx-controller.ingress-nginx.svc.cluster.local/api/v1/campaigns -host api.$(DOMAIN) -insecure -recipients $$N_VAL | kubectl --kubeconfig $(KUBECONFIG_LOCAL) create -f -

load-api: ## Запустить нагрузочное HTTP-тестирование API через Kubernetes Job (RPS=300, DURATION=2m)
	@RPS_VAL=$${RPS:-300}; \
	DUR_VAL=$${DURATION:-2m}; \
	EPOCH=$$(date +%s); \
	scripts/loadgen-job.sh loadgen-api-$$EPOCH $(IMAGE) http -url https://ingress-nginx-controller.ingress-nginx.svc.cluster.local/api/v1/campaigns -host api.$(DOMAIN) -insecure -rps $$RPS_VAL -duration $$DUR_VAL | kubectl --kubeconfig $(KUBECONFIG_LOCAL) create -f -

demo-canary: ## Запустить canary-релиз notifier-api через values.yaml (api.releaseNote)
	@TS=$$(date -u +"%Y-%m-%dT%H:%M:%SZ"); \
	echo "Выполняется canary-релиз с releaseNote: demo-$$TS..."; \
	if command -v yq >/dev/null 2>&1; then \
		yq -i ".api.releaseNote = \"demo-$$TS\"" deploy/apps/notifier/values.yaml; \
	else \
		sed -i.bak -E "s/releaseNote: .*/releaseNote: \"demo-$$TS\"/" deploy/apps/notifier/values.yaml && rm -f deploy/apps/notifier/values.yaml.bak; \
	fi; \
	git add deploy/apps/notifier/values.yaml && \
	git commit -m "demo: canary release $$TS" && \
	git push; \
	echo ""; \
	echo "Canary-релиз отправлен в git! Что отслеживать:"; \
	echo "  1. Статус Rollout: make rollout-status"; \
	echo "  2. Дашборд Grafana: https://grafana.$(DOMAIN) (панели Canary vs Stable)"; \
	echo "  3. Прогресс канарейки в ArgoCD: https://argocd.$(DOMAIN)"

demo-rollback: ## Запустить canary-релиз с ошибками (FAULT_ERROR_RATE=0.5) для автоотката
	@echo "Устанавливается faultErrorRate=0.5 для демонстрации отката..."; \
	if command -v yq >/dev/null 2>&1; then \
		yq -i '.api.faultErrorRate = "0.5"' deploy/apps/notifier/values.yaml; \
	else \
		sed -i.bak -E 's/faultErrorRate: .*/faultErrorRate: "0.5"/' deploy/apps/notifier/values.yaml && rm -f deploy/apps/notifier/values.yaml.bak; \
	fi; \
	git add deploy/apps/notifier/values.yaml && \
	git commit -m "demo: bad release (50% errors)" && \
	git push; \
	echo ""; \
	echo "Релиз с 50% ошибок отправлен в git!"; \
	echo "Ожидаемое поведение:"; \
	echo "  1. Rollout переводит 20% трафика на canary"; \
	echo "  2. AnalysisTemplate notifier-api-health фиксирует success-rate < 0.95"; \
	echo "  3. Rollout переходит в статус Degraded/Aborted"; \
	echo "  4. 100% трафика возвращается на stable (инцидент предотвращен)"; \
	echo "  Проверить статус: make rollout-status"

demo-fix: ## Сбросить faultErrorRate обратно в 0 и вернуть Rollout в здоровое состояние
	@echo "Сброс faultErrorRate в 0..."; \
	if command -v yq >/dev/null 2>&1; then \
		yq -i '.api.faultErrorRate = "0"' deploy/apps/notifier/values.yaml; \
	else \
		sed -i.bak -E 's/faultErrorRate: .*/faultErrorRate: "0"/' deploy/apps/notifier/values.yaml && rm -f deploy/apps/notifier/values.yaml.bak; \
	fi; \
	git add deploy/apps/notifier/values.yaml && \
	git commit -m "demo: fix bad release (0% errors)" && \
	git push; \
	echo ""; \
	echo "Исправление отправлено в git! Rollout успешно выкатит новую версию на 100%."

rollout-status: ## Показать статус Rollout notifier-api и список AnalysisRuns
	@kubectl --kubeconfig $(KUBECONFIG_LOCAL) -n notifier get rollout notifier-api -o wide
	@echo ""
	@kubectl --kubeconfig $(KUBECONFIG_LOCAL) -n notifier get analysisruns

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
