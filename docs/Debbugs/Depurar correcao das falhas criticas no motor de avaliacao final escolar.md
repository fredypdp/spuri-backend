---
data: 2026-08-20
status: parcial_sem_postgres
---

# Depurar correção das falhas críticas no motor de avaliação final escolar

## O que foi corrigido

1. O gatilho automático agora é sensível ao período de fechamento da fórmula, evitando fechamento prematuro por `prova_trimestral` do 1º ou 2º trimestre.
2. O cálculo automático escolar deixou de avaliar apenas a matéria que recebeu a nota recém-lançada. A avaliação só é registrada quando todas as matérias aplicáveis possuem as referências exigidas pela fórmula, e a decisão geral/progressão usa o conjunto completo.
3. O 4º ano médio técnico passou a ter ancoragem possível para PAP: matéria média com `anos_academicos: ["4_ano_medio"]` é aceita somente em curso médio técnico.

## Validado sem banco de dados

Foram executados testes unitários Go sem PostgreSQL cobrindo gatilho por período, completude da fórmula, manutenção da substituição por zero, PAP técnico e progressão final de curso médio.

## Pendente de validação com PostgreSQL real

O ambiente atual não possui Docker, `psql` nem Postgres local. Ficam pendentes os cenários HTTP end-to-end abaixo:

- Fundamental sem exame com 2+ matérias, garantindo que 1º/2º trimestres não disparam fechamento e que o 3º trimestre só fecha quando todas as matérias estiverem completas.
- Fundamental com exame e recurso, incluindo matérias aprovadas e reprovadas em cadeias normal → exame/recurso.
- Médio liceu nos anos 1º, 2º e 3º, confirmando que liceu não aceita nem gera 4º ano médio.
- Médio técnico nos anos 1º a 4º, incluindo criação de matéria PAP, lançamento de `nota_pap`, aprovação, reprovação e conclusão do curso técnico.
- Correção de nota após existência de avaliação de outro tipo/ano, confirmando que o comportamento idempotente completo permanece consistente.
- Concorrência/ordem de chegada com 3+ matérias, confirmando que o resultado final independe da primeira matéria cujo gatilho chegou.
