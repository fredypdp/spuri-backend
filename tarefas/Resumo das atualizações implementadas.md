---
criado: 2026-06-28
origem: resumo das atualizações implementadas
status: concluido
---

# Resumo das atualizações implementadas

Este documento resume, em linguagem direta para o cliente, as principais mudanças entregues nas atualizações recentes.

## 1. Remoção do número de telefone extra

- O recurso antigo de **telefones extras** foi removido do sistema.
- Estudantes, academias e administradores passaram a usar campos de telefone nativos e padronizados.
- O telefone da academia foi padronizado para o campo `telefone`.
- Estudantes agora possuem telefone próprio e telefone do responsável, com regra para garantir que pelo menos um telefone seja informado quando aplicável.
- Os telefones são normalizados antes de salvar, removendo espaços, hífens e parênteses, mantendo o formato local de 9 dígitos.
- Os campos de verificação de telefone foram mantidos apenas como preparação futura; ainda não existe fluxo ativo de verificação de telefone.

## 2. Sumários/aulas e vínculo opcional às faltas

- Foi adicionado o cadastro de **sumários/aulas** por academia.
- A academia pode registrar informações da aula, como título, descrição, período, ano acadêmico, curso e matéria.
- As faltas podem ser vinculadas opcionalmente a um sumário/aula por `sumario_id`.
- Quando uma falta é vinculada a um sumário, o sistema guarda também o título da aula como histórico, preservando a informação mesmo se o sumário for editado depois.
- Foram adicionadas validações para impedir vínculo de faltas com sumários de outra academia ou incompatíveis com o contexto acadêmico informado.

## 3. Cadastro direto de estudante pela academia com documentação completa

- O cadastro direto de estudante pela academia foi alinhado ao fluxo de solicitação de matrícula.
- O cadastro direto passou a exigir envio por `multipart/form-data` quando houver documentos obrigatórios.
- Foram incluídas validações para documentos como BI do estudante, BI do responsável, cédula do estudante, declaração e certificados acadêmicos aplicáveis.
- Os documentos enviados são armazenados e associados ao cadastro do estudante.
- A criação direta do estudante só é concluída depois que as informações e documentos obrigatórios forem validados.

## 3.1. BI do responsável obrigatório para estudante escolar

- Estudantes escolares/fundamental/médio agora precisam informar o `bilhete_identidade_responsavel` nos fluxos de criação e aprovação.
- O PDF do BI do responsável também é obrigatório para estudantes escolares.
- Quando o estudante possui BI próprio, o PDF do BI do estudante deve ser enviado; quando não possui, deve ser enviada a cédula do estudante conforme a regra aplicável.
- O BI do estudante não pode ser igual ao BI do responsável do mesmo estudante.
- O sistema impede conflitos entre o BI do responsável e o BI principal de outro estudante escolar/fundamental/médio.
- A aprovação de solicitação de matrícula revalida essas regras antes de criar o estudante.

## 4. Curso superior com períodos numéricos e anos acadêmicos calculados

- Cursos superiores passaram a ser configurados pela quantidade numérica de períodos/semestres.
- O sistema calcula automaticamente os semestres (`1_semestre`, `2_semestre`, etc.) a partir do número informado.
- Os anos acadêmicos do curso superior também são derivados automaticamente, usando a relação de dois semestres por ano acadêmico.
- O cliente não precisa mais enviar manualmente listas de semestres ou anos superiores.
- Foram adicionadas validações para impedir valores inválidos, listas manuais e configurações incompatíveis com curso superior.

## 4.1. Academias podem adicionar ou remover anos acadêmicos com validações avançadas

- Foram adicionadas rotas para consultar e gerir anos acadêmicos disponíveis na academia.
- A academia pode habilitar novos anos acadêmicos compatíveis com o seu nível de ensino.
- A remoção é lógica, ou seja, desativa a oferta futura sem apagar histórico já existente.
- O sistema bloqueia alterações quando existirem estudantes ativos, turmas, matérias, notas, faltas, sumários ou outros dados dependentes que tornem a mudança insegura.
- Para cursos superiores, a gestão continua baseada na quantidade de períodos do curso; os anos superiores permanecem calculados automaticamente.
- As alterações respeitam o escopo da academia autenticada, evitando que uma academia altere configurações de outra.

## 4.2. Fundamental permanece em andamento quando aprovado sem próximo ano ofertado pela academia

- Quando um estudante do fundamental é aprovado, o sistema diferencia o fim real do ensino fundamental da falta de oferta do próximo ano pela academia.
- Se ainda existe próximo ano fundamental na sequência global, mas a academia não oferece esse ano, o estudante não é finalizado indevidamente.
- Nessa situação, o estudante permanece com status `em_andamento` e avança para o próximo ano acadêmico global.
- O sistema não cria automaticamente turma, matéria, curso ou outro vínculo para um ano que a academia ainda não oferece.
- O histórico de avaliação permanece registrado normalmente.

## 4.3. Preservação de histórico acadêmico ao desvincular ou inativar estudante

- A inativação, interrupção, trancamento ou desvinculação de um estudante não apaga nem reinicia seu histórico acadêmico.
- Ao retornar para o mesmo curso ou trajetória acadêmica, o estudante mantém a progressão anterior.
- Se houver mudança real de curso superior, o novo vínculo começa no `1_semestre` e no `1_ano_superior`, preservando o histórico anterior.
- Se houver mudança real de curso médio, o novo vínculo começa no `1_ano_medio`, também sem apagar o histórico anterior.
- Notas, faltas, avaliações, turmas, documentos, mensalidades, ledger e demais registros históricos permanecem consultáveis.
