---
modificado: 2026-06-28 2:14
criado: 2026-06-28 2:14
---
# Curso superior: períodos numéricos e anos acadêmicos calculados

## Objetivo

Adaptar a criação e edição de cursos superiores para que o cliente informe apenas a quantidade total de semestres do curso em `periodos`, agora como número, e para que o backend calcule automaticamente os anos acadêmicos superiores equivalentes.

## Estado atual observado

Hoje os cursos usam `anos_academicos` como lista de strings e `periodos` como lista de semestres no formato textual, por exemplo `['1_semestre', '2_semestre']`. Para cursos superiores, isso permite que o payload envie manualmente tanto os anos acadêmicos quanto os semestres ofertados.

A avaliação final do estudante superior já possui lógica de progressão semestral: o backend incrementa `semestre_atual` quando existe próximo semestre no curso e recalcula `ano_superior` com base no semestre aprovado. Essa regra deve ser reaproveitada como referência para derivar os anos acadêmicos do curso a partir do total de semestres.

## Regra de negócio recomendada

### Para cursos superiores

- `periodos`: obrigatório e deve ser um número inteiro positivo.
- `periodos` representa a quantidade total de semestres do curso.
- Não deve ser mais aceito o envio de `anos_academicos` no payload de criação ou edição de curso superior.
- O backend deve calcular `anos_academicos` automaticamente a partir de `periodos`.
- O cálculo deve considerar que cada ano acadêmico superior equivale a até 2 semestres.
- Fórmula recomendada: `total_anos = ceil(periodos / 2)`.
- Exemplo:
  - `periodos = 1` → `anos_academicos = ['1_ano_superior']`
  - `periodos = 2` → `anos_academicos = ['1_ano_superior']`
  - `periodos = 3` → `anos_academicos = ['1_ano_superior', '2_ano_superior']`
  - `periodos = 4` → `anos_academicos = ['1_ano_superior', '2_ano_superior']`
  - `periodos = 6` → `anos_academicos = ['1_ano_superior', '2_ano_superior', '3_ano_superior']`

### Para cursos médios/escolares

- Manter o comportamento atual de `anos_academicos`, quando aplicável ao tipo do curso.
- `periodos` numérico deve ser aceito apenas para curso superior.
- Não misturar a regra de semestres superiores com cursos do ensino médio.

## Ajuste necessário

Alterar os DTOs, validações e handlers de criação/edição de cursos para suportar a nova forma do payload superior:

```json
{
  "nome": "Engenharia Informática",
  "type": "superior",
  "periodos": 8
}
```

O backend deve persistir os semestres derivados no formato já usado internamente quando necessário para compatibilidade com as regras atuais, por exemplo:

```json
{
  "periodos": ["1_semestre", "2_semestre", "3_semestre", "4_semestre", "5_semestre", "6_semestre", "7_semestre", "8_semestre"],
  "anos_academicos": ["1_ano_superior", "2_ano_superior", "3_ano_superior", "4_ano_superior"]
}
```

Se a decisão técnica for migrar também o modelo persistido para `periodos` numérico, criar migração e adaptar todos os consumidores que hoje esperam lista de strings. Caso contrário, manter a persistência textual e tratar o número apenas como contrato de entrada da API.

## Validações

- Rejeitar curso superior sem `periodos`.
- Rejeitar `periodos <= 0`.
- Rejeitar `periodos` decimal, string, array ou valor nulo para curso superior.
- Rejeitar payload de curso superior que envie `anos_academicos`.
- Rejeitar tentativa de edição de curso superior que reduza `periodos` removendo semestre já usado por estudante ativo em `semestre_atual`.
- Rejeitar tentativa de edição que reduza os anos acadêmicos derivados removendo ano ainda usado por estudante ativo, se houver validação equivalente no modelo atual.
- Garantir que os semestres derivados sejam sempre sequenciais, de `1_semestre` até `N_semestre`.
- Garantir que os anos derivados sejam sempre sequenciais, de `1_ano_superior` até `ceil(N/2)_ano_superior`.
- Não aceitar strings vazias ou campos ignorados silenciosamente quando indiquem uso do contrato antigo.

## Fluxo operacional na criação de curso superior

1. Academia envia `nome`, `type='superior'` e `periodos` numérico.
2. Backend valida que `anos_academicos` não foi enviado.
3. Backend valida que `periodos` é inteiro positivo.
4. Backend gera a lista de semestres internos de `1_semestre` até `N_semestre`.
5. Backend calcula `anos_academicos` com `ceil(N / 2)`.
6. Backend grava o evento de criação do curso já com os valores derivados.
7. Projeção do curso mantém resposta consistente para listagem e detalhe.

## Fluxo operacional na edição de curso superior

1. Academia envia novo `periodos` numérico, quando quiser alterar a duração do curso.
2. Backend bloqueia payload com `anos_academicos` para curso superior.
3. Backend recalcula semestres e anos acadêmicos derivados.
4. Se houver redução, backend valida se algum estudante ativo usa semestre/ano que seria removido.
5. Backend grava evento de atualização somente após validar compatibilidade com estudantes ativos.
6. Projeção atualiza `periodos` e `anos_academicos` derivados.

## Impactos esperados

- Contrato da API de cursos superiores muda de `periodos: string[]` para `periodos: number` no payload de criação/edição.
- Clientes não precisam mais conhecer nem enviar `anos_academicos` superiores.
- O backend passa a ser a fonte única para mapear semestres em anos superiores.
- Documentação da API deve ser atualizada nos exemplos e mensagens de erro de cursos.
- Testes existentes que criam curso superior com `periodos` array ou `anos_academicos` no payload devem ser atualizados.

## Testes recomendados

- Criar curso superior com `periodos=1`: deve criar `1_semestre` e `1_ano_superior`.
- Criar curso superior com `periodos=2`: deve criar `1_semestre`, `2_semestre` e apenas `1_ano_superior`.
- Criar curso superior com `periodos=3`: deve criar 3 semestres e `1_ano_superior`, `2_ano_superior`.
- Criar curso superior enviando `anos_academicos`: deve falhar.
- Criar curso superior com `periodos` como array antigo: deve falhar.
- Criar curso superior com `periodos=0`, negativo, decimal ou string: deve falhar.
- Editar curso superior aumentando `periodos`: deve recalcular semestres e anos acadêmicos.
- Editar curso superior reduzindo `periodos` sem estudantes ativos nos semestres removidos: deve passar.
- Editar curso superior reduzindo `periodos` com estudante ativo em semestre removido: deve falhar.
- Confirmar que aprovação final superior continua progredindo `semestre_atual` e `ano_superior` corretamente com o curso configurado por quantidade de semestres.
