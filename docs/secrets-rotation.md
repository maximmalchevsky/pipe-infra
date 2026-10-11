# Управление и ротация секретов (Sealed Secrets)

В проекте используется подход GitOps для безопасного хранения секретов в открытом репозитории с помощью [Bitnami Sealed Secrets](https://github.com/bitnami-labs/sealed-secrets).

---

## 1. Архитектура и жизненный цикл секретов

Секреты шифруются асимметричным шифрованием (RSA 4096). Публичный сертификат используется локально для шифрования, а приватный ключ хранится исключительно в кластере и локально в защищенном каталоге `.secrets/`.

```
+-------------------------------------------------------------------------+
|                                Разработчик                              |
|                                                                         |
|   .env (Plaintext)                                                      |
|          |                                                              |
|          v                                                              |
|   make seal ------------+                                               |
|   (kubectl + kubeseal)  |                                               |
|          ^              |                                               |
|          |              v                                               |
|   .secrets/*.crt   deploy/platform/secrets/<ns>/<name>.sealed.yaml      |
+---------------------------------|---------------------------------------+
                                  | git push
                                  v
+-------------------------------------------------------------------------+
|                                 GitOps                                  |
|                                                                         |
|                          GitHub Repository                              |
|                                 |                                       |
|                                 v                                       |
|                         ArgoCD Application                              |
|                         (platform-secrets)                              |
+---------------------------------|---------------------------------------+
                                  | sync
                                  v
+-------------------------------------------------------------------------+
|                           Kubernetes Кластер                            |
|                                                                         |
|    kube-system:                                                         |
|      Secret: sealed-secrets-key-hackathon (tls.key + tls.crt)           |
|                         |                                               |
|                         v                                               |
|      Pod: sealed-secrets-controller                                     |
|                         |                                               |
|                         | расшифровывает SealedSecret                   |
|                         v                                               |
|    <namespace>:                                                         |
|      Secret: <name> (обычный Kubernetes Secret)                         |
|                         |                                               |
|                         v                                               |
|      Используется приложениями (Grafana, backend API и др.)             |
+-------------------------------------------------------------------------+
```

### Где хранятся данные:
- **Локальный plaintext-источник:** `.env` (файл добавлен в `.gitignore`, права `0600`).
- **Локальная ключевая пара:** `.secrets/sealed-secrets.key` (права `0600`) и `.secrets/sealed-secrets.crt` (права `0644`). Каталог `.secrets/` добавлен в `.gitignore`.
- **Зашифрованные манифесты:** `deploy/platform/secrets/<namespace>/<name>.sealed.yaml` (безопасны для коммита в публичный Git).
- **Мастер-ключ в кластере:** Kubernetes Secret `sealed-secrets-key-hackathon` в namespace `kube-system` с меткой `sealedsecrets.bitnami.com/sealed-secrets-key: active`. Создается ролью Ansible `sealed_secrets_key`.

---

## 2. Ротация прикладных секретов (например, пароля Grafana)

Если требуется изменить пароль администратора Grafana или секрет любого сервиса:

1. **Обновить значение в `.env`:**
   Измените соответствующую переменную, например:
   ```bash
   # в файле .env
   GRAFANA_ADMIN_PASSWORD=новый_сложный_пароль
   ```

2. **Запустить перешифрование:**
   ```bash
   make seal
   ```
   Команда прочитает `.env` построчно (без выполнения `source`), сгенерирует Secret через `kubectl --dry-run=client` и зашифрует его утилитой `kubeseal` с открытым ключом `.secrets/sealed-secrets.crt`. Результат запишется в `deploy/platform/secrets/monitoring/grafana-admin.sealed.yaml`.

3. **Проверить diff в Git:**
   ```bash
   git diff deploy/platform/secrets/
   ```
   Убедитесь, что изменились только поля `spec.encryptedData`. Открытый пароль ни в коем случае не должен присутствовать в файле.

4. **Закоммитить и отправить в репозиторий:**
   ```bash
   git add deploy/platform/secrets/
   git commit -m "chore: rotate grafana admin secret"
   git push
   ```

5. **Синхронизация и применение:**
   - ArgoCD автоматически подхватит изменения через приложение `platform-secrets` (sync-wave "1").
   - Контроллер `sealed-secrets-controller` обновит целевой Secret `grafana-admin` в namespace `monitoring`.
   - Если приложение не отслеживает динамическое изменение секретов на лету, выполните перезапуск его рабочей нагрузки:
     ```bash
     kubectl --kubeconfig .kube/config -n monitoring rollout restart deployment grafana
     ```

---

## 3. Плановая ротация мастер-ключа шифрования (Sealing Key)

Для обеспечения высокой безопасности рекомендуется периодически ротировать сам ключ шифрования Sealed Secrets:

1. **Создание новой ключевой пары:**
   Создайте новый ключ и сертификат, например:
   ```bash
   openssl req -x509 -nodes -newkey rsa:4096 \
     -keyout .secrets/sealed-secrets-new.key \
     -out .secrets/sealed-secrets-new.crt \
     -subj "/CN=sealed-secret" \
     -days 3650
   ```

2. **Добавление нового ключа в кластер:**
   В кластере может присутствовать несколько активных ключей одновременно. Создайте новый Secret в `kube-system`:
   ```bash
   kubectl --kubeconfig .kube/config -n kube-system create secret tls sealed-secrets-key-v2 \
     --cert=.secrets/sealed-secrets-new.crt \
     --key=.secrets/sealed-secrets-new.key
   kubectl --kubeconfig .kube/config -n kube-system label secret sealed-secrets-key-v2 \
     sealedsecrets.bitnami.com/sealed-secrets-key=active
   ```

3. **Перешифрование всех секретов:**
   Замените `.secrets/sealed-secrets.crt` и `.secrets/sealed-secrets.key` новыми файлами и запустите:
   ```bash
   make seal
   ```
   Либо воспользуйтесь встроенной командой:
   ```bash
   kubeseal --re-encrypt --cert .secrets/sealed-secrets.crt < old.sealed.yaml > new.sealed.yaml
   ```

4. **Коммит и синхронизация:**
   Закоммитьте обновленные `.sealed.yaml` файлы в Git.

5. **Удаление старого приватного ключа:**
   После того как все SealedSecret перешифрованы и успешно расшифровываются новым ключом, удалите старый Secret из кластера:
   ```bash
   kubectl --kubeconfig .kube/config -n kube-system delete secret sealed-secrets-key-hackathon
   ```

---

## 4. Действия при компрометации приватного ключа

Если приватный ключ `.secrets/sealed-secrets.key` попал в открытый доступ или был скомпрометирован:

1. **НЕМЕДЛЕННО удалить скомпрометированный Secret из кластера:**
   ```bash
   kubectl --kubeconfig .kube/config -n kube-system delete secret sealed-secrets-key-hackathon
   ```
   Это предотвратит дешифрование новых секретов с использованием скомпрометированного ключа.

2. **Сгенерировать абсолютно новую пару ключей:**
   ```bash
   rm -f .secrets/sealed-secrets.key .secrets/sealed-secrets.crt
   make sealing-key
   ```

3. **Сменить значения ВСЕХ секретов в `.env`:**
   Поскольку злоумышленник, получивший приватный ключ и доступ к публичному Git-репозиторию, мог расшифровать все секреты, старые значения считаются скомпрометированными.
   - Сгенерируйте новые пароли баз данных, токены и пароли сервисов.

4. **Перешифровать и доставить новые секреты:**
   ```bash
   make seal
   ```
   Закоммитьте и запушьте новые зашифрованные манифесты.

5. **Обновить ключ в кластере:**
   Загрузите новый Secret в `kube-system` через Ansible (`ansible-playbook infra/ansible/site.yml`) или вручную, после чего перезапустите контроллер:
   ```bash
   kubectl --kubeconfig .kube/config -n kube-system rollout restart deployment sealed-secrets-controller
   ```

---

## 5. Резервное копирование `.secrets/`

Поскольку файл `.secrets/sealed-secrets.key` не хранится в Git, при его потере невозможно будет восстановить зашифрованные секреты без повторной ручной генерации паролей:
- Сохраняйте резервную копию каталога `.secrets/` в защищенном хранилище паролей команды (например, 1Password, Bitwarden, HashiCorp Vault или в виде GPG-зашифрованного архива).
- Резервная копия необходима для развертывания нового кластера с уже закоммиченными `.sealed.yaml` манифестами без необходимости перешифровывать весь репозиторий.

---

## 6. Примечание по инфраструктурным секретам (Proxmox)

Ротация API-токена Proxmox VE (`PROXMOX_VE_API_TOKEN`) и паролей суперпользователя виртуальных машин запланирована в рамках этапа 10 (stage 10).
