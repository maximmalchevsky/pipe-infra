# Настройка Telegram-уведомлений (Alertmanager и Argo Rollouts)

Данный документ описывает процесс настройки и проверки отправки уведомлений в Telegram для двух компонентов платформы:
1. **Alertmanager** (мониторинг критических событий кластера и алертов приложений, namespace `monitoring`).
2. **Argo Rollouts** (события жизненного цикла Canary-развертываний: завершение, аборт/откат, ошибки AnalysisRun, namespace `argo-rollouts`).

Все конфигурации закрыты флагом `telegram.enabled: false` в `deploy/root/values.yaml`. До тех пор пока переменные не заданы в `.env` и флаг выключен, манифесты кластера остаются без изменений.

---

## Шаг 1. Создание бота в Telegram и получение токена

1. Откройте Telegram и найдите официального бота `@BotFather`.
2. Отправьте команду `/newbot`.
3. Задайте имя бота (например, `Pipe Cluster Notifier`) и уникальный юзернейм (например, `pipe_cluster_notifier_bot`).
4. Скопируйте полученный HTTP API токен вида `1234567890:ABCdefGHIjklMNOpqrsTUVwxyz`.

---

## Шаг 2. Получение ID чата или группы (Chat ID)

### Вариант А. Личные сообщения с ботом
1. Нажмите "Start" в диалоге с созданным ботом.
2. Узнайте свой персональный ID через бота `@userinfobot` или отправьте боту любое сообщение и выполните запрос:
   ```bash
   curl -s https://api.telegram.org/bot<ВАШ_ТОКЕН>/getUpdates | grep -o '"chat":{"id":[-0-9]*'
   ```

### Вариант Б. Группа или супергруппа
1. Создайте группу в Telegram и добавьте в нее созданного бота.
2. Назначьте бота администратором группы (с правом отправки сообщений).
3. Напишите любое сообщение в группу.
4. Выполните запрос:
   ```bash
   curl -s https://api.telegram.org/bot<ВАШ_ТОКЕН>/getUpdates | grep -o '"chat":{"id":[-0-9]*'
   ```
   ID группы обычно начинается с минуса (например, `-1001234567890`).

---

## Шаг 3. Добавление переменных в .env

В корне проекта откройте файл `.env` и добавьте (или заполните) параметры:

```bash
# Токен бота Telegram от @BotFather (секрет)
TELEGRAM_BOT_TOKEN="1234567890:ABCdefGHIjklMNOpqrsTUVwxyz"

# Chat ID чата или группы (не секрет)
TELEGRAM_CHAT_ID="-1001234567890"
```

ВНИМАНИЕ:
- Никогда не коммитьте `.env` в git.
- Никогда не пересылайте токен бота в чаты или тикеты.

---

## Шаг 4. Запечатывание секретов (Sealed Secrets)

Выполните генерацию запечатанных секретов:

```bash
make seal
```

Скрипт `scripts/seal-secrets.sh`:
- Прочитает `TELEGRAM_BOT_TOKEN` из `.env`.
- Создаст запечатанный секрет `alertmanager-telegram` (ключ `bot_token`) в namespace `monitoring`.
- Создаст запечатанный секрет `argo-rollouts-notification-secret` (ключ `telegram-token`) в namespace `argo-rollouts` с меткой компонента `app.kubernetes.io/component: rollouts-controller`.
- Не будет создавать отдельный дублирующий манифест Namespace для `argo-rollouts`, так как этот namespace управляется приложением `argo-rollouts`.
- Оставит существующие запечатанные секреты неизменными, если их данные не менялись.

---

## Шаг 5. Включение уведомлений в GitOps

Откройте `deploy/root/values.yaml` и измените блок `telegram`:

```yaml
telegram:
  enabled: true
  chatId: "-1001234567890"
```

Зафиксируйте изменения в git и отправьте в репозиторий:

```bash
git add deploy/platform/secrets/ deploy/root/values.yaml
git commit -m "feat: enable telegram notifications for alertmanager and argo-rollouts"
git push origin main
```

ArgoCD автоматически применит запечатанные секреты (`platform-secrets`), обновит конфигурацию `kube-prometheus-stack` и `argo-rollouts`.

---

## Шаг 6. Проверка работоспособности

### 1. Проверка Alertmanager
Для проверки отправки алертов можно временно симулировать недоступность API сервиса:

1. Масштабируйте реплики в ноль:
   ```bash
   kubectl --kubeconfig .kube/config -n notifier scale rollout notifier-api --replicas=0
   ```
2. В течение 1-2 минут Prometheus зафиксирует отсутствие эндпоинтов и сгенерирует алерт `NotifierApiDown`.
3. Alertmanager отправит уведомление в Telegram с деталями алерта и ссылкой.
4. Восстановите количество реплик:
   ```bash
   kubectl --kubeconfig .kube/config -n notifier scale rollout notifier-api --replicas=2
   ```
5. Alertmanager пришлет сообщение о разрешении инцидента (`send_resolved: true`).

### 2. Проверка Argo Rollouts
Для проверки уведомлений о событиях роллаута:

1. Запустите прерывание текущего роллаута или демонстрационный откат:
   ```bash
   kubectl --kubeconfig .kube/config argo rollouts abort notifier-api -n notifier
   ```
2. В Telegram поступит уведомление формата:
   ```text
   [ABORTED] Rollout notifier-api in namespace notifier was aborted!
   Image: ghcr.io/.../notifier-api:...
   ```
3. Для возврата роллаута в нормальное состояние:
   ```bash
   kubectl --kubeconfig .kube/config argo rollouts retry rollout notifier-api -n notifier
   ```
4. После успешного обновления поступит уведомление:
   ```text
   [OK] Rollout notifier-api in namespace notifier completed successfully.
   Image: ghcr.io/.../notifier-api:...
   ```

---

## Информация для владельцев чартов приложений

Благодаря настройке централизованного блока `subscriptions` в `argo-rollouts-notification-configmap`, все Rollout-манифесты (включая `notifier-api`) автоматически подписаны на события:
- `on-rollout-completed`
- `on-rollout-aborted`
- `on-analysis-run-failed`

Разработчикам приложения `notifier` **не требуется** модифицировать чарт `deploy/apps/notifier/`.

Если для отдельного Rollout потребуется переопределить получателя или подписаться на дополнительные события, это можно сделать через стандартные аннотации в манифесте Rollout:
```yaml
metadata:
  annotations:
    notifications.argoproj.io/subscribe.on-rollout-aborted.telegram: "<CHAT_ID>"
    notifications.argoproj.io/subscribe.on-rollout-completed.telegram: "<CHAT_ID>"
    notifications.argoproj.io/subscribe.on-analysis-run-failed.telegram: "<CHAT_ID>"
```
