---
concluido: 2026-07-15
origem: docs/Lista de tarefas/Separar documentos academicos de estudante por nivel e ano.md
status: implementado
---

# Separar documentos acadêmicos de estudante por nível e ano (feito)

## Implementação

- `DocumentoMatricula` agora carrega metadados normalizados (`documento_id`, `tipo`, `nivel`, `ano_academico`, `curso_id`, `ano_letivo`, `versao`, `path`, `file_url`, `download_url`).
- Declarações deixam de ser persistidas como tipo genérico nos novos uploads e nos documentos enviados via JSON no cadastro direto; o `tipo` passa a ser `declaracao_<ano_academico>`, por exemplo `declaracao_3_ano_medio`.
- Uploads de cadastro direto, solicitação de matrícula e conclusão de documentos pendentes gravam arquivos em paths com escopo acadêmico, por exemplo `.../documentos/medio/3_ano_medio/declaracao_3_ano_medio/<documento_id>.pdf`.
- Documentos de identificação permanecem em `identificacao/<tipo>/<documento_id>.pdf`.
- O aggregate e a projeção de estudante passam a mesclar documentos completados em vez de substituir o conjunto completo.
- A migration `090_normalizar_documentos_academicos_estudante.sql` reindexa o JSON legado preservando metadados e marcando escopos não inferíveis como `escopo_desconhecido`.

## Contrato de persistência e resposta

- Documentos de identificação usam a chave do próprio tipo (`bi_estudante`, `bi_responsavel`, `cedula_estudante`) e path `identificacao/<tipo>/<documento_id>.pdf`.
- Documentos acadêmicos usam chave composta `nivel.ano_academico.tipo`, por exemplo `medio.3_ano_medio.declaracao_3_ano_medio`.
- Declarações e certificados persistidos retornam metadados normalizados: `documento_id`, `tipo`, `nivel`, `ano_academico`, `curso_id`, `ano_letivo`, `versao`, `path`, `file_url` e `download_url`.
- Documentos acadêmicos sem escopo inferível são isolados sob `escopo_desconhecido.<tipo>` para não sobrescrever documentos válidos nem ressuscitar a chave raiz genérica `declaracao`.
- O cadastro direto normaliza também `req.Documentos` antes da validação e antes da persistência, mantendo o mesmo contrato dos uploads multipart.

## Validação

- A validação acadêmica procura documentos por `tipo` explícito e aceita `declaracao_3_ano_medio` ou `certificado_ensino_medio` para ingresso no `1_ano_superior`.
- `1_ano_fundamental` permanece sem exigência de comprovativo acadêmico anterior.
- A nomenclatura `declaracao_ensino_medio` não foi introduzida no código, migrations ou documentação técnica nova.
