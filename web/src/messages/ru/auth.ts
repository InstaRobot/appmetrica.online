import type { auth as en } from "../en/auth";

export const auth: typeof en = {
  login: {
    metaTitle: "Вход",
    title: "Вход в appmetrica.online",
    hint: "Используйте passkey на этом устройстве или телефоне.",
    setupRequired:
      "Passkey ещё не настроен. На сервере выполните {command} и откройте выведенную ссылку.",
  },
  setup: {
    metaTitle: "Регистрация passkey",
    title: "Регистрация passkey",
    oneTime: "Ссылка одноразовая и действует 15 минут.",
    defaultDeviceName: "Устройство",
    deviceName: "Название устройства",
    submit: "Создать passkey",
    submitting: "Создаём passkey…",
    success: "Passkey создан. Переходим в дашборд…",
    expiredTitle: "Ссылка устарела или уже использована",
    expiredHint: "Получите новую: выполните на сервере {command}.",
  },
  passkey: {
    signIn: "Войти с passkey",
    waiting: "Ожидаем подтверждение…",
    unsupported: "Браузер не поддерживает passkey",
    help: "Где работают passkey",
  },
  deviceName: {
    required: "Введите название устройства",
    tooLong: "Не длиннее 64 символов",
  },
  passkeyErrors: {
    login: "Не удалось войти. Подтверждение отменено или ключ не подошёл. Попробуйте ещё раз.",
    setup: "Не удалось создать passkey. Попробуйте ещё раз.",
    network: "Нет соединения с сервером. Проверьте подключение.",
    rateLimited: "Слишком много попыток. Повторите через минуту.",
    sessionExpired: "Сессия устарела. Обновите страницу.",
  },
};
