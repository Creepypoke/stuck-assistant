# Сравнение промптов

Текущий `prompt-eval` принимает произвольный корпус, набор вопросов и локальную
модель, самостоятельно собирает кеш и manifest; ограничение ровно 20 вопросов
и зависимость от прежнего tmp-каталога удалены. По умолчанию сравниваются
`default` и `strict-ru`; также сохранены `strict`, `strict-examples` и
`strict-examples-ru` для воспроизведения предыдущих экспериментов.

Команды, oracle-context, cache identity и ограничения: [P0 evaluation](../README.md).
Исторические результаты на 4B:
[профили](../reports/QWEN-PROMPT-COMPARISON-REPORT.md),
[язык инструкций](../reports/QWEN-PROMPT-LANGUAGE-COMPARISON-REPORT.md).
