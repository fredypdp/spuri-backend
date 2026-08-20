---
concluido: 2026-08-20
origem: docs/Lista de Tarefas/57 - Corrigir falhas criticas no motor de avaliacao final escolar (aprovacao e reprovacao).md
status: concluido
---

# Corrigir falhas críticas no motor de avaliação final escolar

## Decisões implementadas

- **Achado 1:** adotada validação explícita do período de fechamento derivado da fórmula. A categoria despertadora só dispara quando o período da nota corresponde ao último período em que essa categoria aparece na fórmula.
- **Achado 2:** em vez de registrar uma avaliação parcial por matéria, a execução automática aguarda que todas as matérias aplicáveis estejam completas para a fórmula e calcula o conjunto inteiro em um único evento idempotente. Isso impede que a primeira matéria consuma sozinha a avaliação do ano/tipo.
- **Achado 3:** adotada a opção A. `4_ano_medio` passa a ser aceito para matéria média apenas quando o curso vinculado é de modelo técnico e o array contém somente esse ano, servindo como ancoradouro da PAP.

## Alterações técnicas

- O gatilho automático passou a receber o período da nota alterada e a comparar esse período com o fechamento calculado a partir da fórmula.
- O cálculo automático deixou de filtrar `resultados_materias` para apenas a matéria do overlay; agora todas as matérias aplicáveis entram no cálculo quando estão prontas.
- Foi adicionada uma checagem pura de completude das referências da fórmula antes do registro automático, preservando as funções de cálculo e substituição por zero.
- A validação HTTP de matéria média permite PAP apenas para curso técnico; o aggregate deixou de bloquear estruturalmente `4_ano_medio`, pois essa restrição depende do modelo do curso e deve ficar no handler.
- A documentação da API foi atualizada na seção 15 para explicar gatilho por período, fechamento com conjunto completo de matérias e criação da matéria PAP.

## Testes adicionados

- Gatilho de `prova_trimestral` não dispara no 1º trimestre e dispara no 3º trimestre.
- Completude da fórmula não remove a substituição por zero existente.
- Progressão de sequência média finaliza liceu no 3º ano e técnico no 4º ano.
- Validação de `4_ano_medio` permite apenas matéria PAP de curso técnico.
