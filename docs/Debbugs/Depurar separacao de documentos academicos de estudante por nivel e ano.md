---
concluido: 2026-07-15
origem: docs/Tarefas feitas/Separar documentos academicos de estudante por nivel e ano.md
status: depurado
---

# Depurar separação de documentos acadêmicos de estudante por nível e ano

## Verificações executadas

- Auditei a normalização dos documentos acadêmicos nos fluxos de solicitação de matrícula, cadastro direto e conclusão de documentos pendentes.
- Confirmei que uploads de `declaracao` já usavam `declaracao_<ano_academico>` e paths com `nivel/ano/tipo`.
- Encontrei uma lacuna no cadastro direto quando documentos eram enviados no corpo JSON (`req.Documentos`): declarações informadas manualmente podiam permanecer sob a chave genérica `declaracao`.

## Correções aplicadas

- Centralizei a normalização em `documentoMatriculaNormalizadoComBase`, preservando metadados recebidos e preenchendo `documento_id`, `tipo`, `nivel`, `ano_academico` e `versao` quando necessário.
- O cadastro direto agora normaliza também documentos informados no corpo da requisição antes da validação e antes da persistência.
- Declarações informadas por upload ou por JSON com `declaracao_ano_academico` passam a usar a chave `nivel.ano_academico.declaracao_<ano_academico>`.
- Declarações sem ano acadêmico deixam de ser gravadas como chave raiz `declaracao` e são isoladas em `escopo_desconhecido.declaracao`.
- Adicionei testes unitários para validar chave normalizada, metadados, path acadêmico e ausência da nomenclatura legada indesejada no código de handlers.

## Resultado

A implementação ficou alinhada ao objetivo de separar documentos acadêmicos por nível e ano, incluindo documentos enviados manualmente no cadastro direto.
