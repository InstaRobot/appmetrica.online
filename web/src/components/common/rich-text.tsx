import { Fragment, type ReactNode } from "react";

const PLACEHOLDER = /\{(\w+)\}/;

// Сообщение словаря с разметкой внутри фразы: плейсхолдеры {name} заменяются узлами —
// richText(m.lead, { event: <code>session_start</code> }). Порядок слов остаётся за словарём языка.
// Без хуков: работает и в серверных, и в клиентских компонентах.
export function richText(message: string, nodes: Record<string, ReactNode>): ReactNode[] {
  // split с группой: чётные элементы — текст, нечётные — имена плейсхолдеров.
  return message.split(PLACEHOLDER).map((part, i) => {
    if (i % 2 === 0) return part;
    // biome-ignore lint/suspicious/noArrayIndexKey: части одного сообщения не переупорядочиваются
    return <Fragment key={i}>{part in nodes ? nodes[part] : `{${part}}`}</Fragment>;
  });
}
