# Cadastro de estudante escolar: B.I. e telefone do responsável

## Objetivo
Garantir que estudantes do nível escolar tenham informações obrigatórias do responsável, especialmente B.I. e telefone, e registrar o que já existe no sistema.

## Situação verificada no código atual
O cadastro direto de estudante por academia possui campos `bilhete_identidade` e `bilhete_identidade_responsavel`, mas não possui campo específico para `telefone_responsavel`. O handler transforma o telefone informado em telefone do próprio estudante, não do responsável.

Na solicitação pública de matrícula, o B.I. do responsável já é obrigatório como texto e também como PDF. O sistema exige `bilhete_identidade_responsavel` e o arquivo `bi_responsavel` antes de concluir a solicitação.

Também existe validação para impedir que o B.I. do estudante e o B.I. do responsável sejam iguais.

## Lacunas identificadas
- Não há campo persistido `telefone_responsavel` em estudante ou solicitação de matrícula.
- Cadastro direto de estudante por academia não obriga B.I. do responsável para estudantes escolares.
- A regra atual permite que o estudante tenha B.I. próprio vazio desde que exista B.I. do responsável em alguns fluxos, mas a demanda pede obrigatoriedade de B.I. para nível escolar; é necessário decidir se isso significa B.I. do estudante ou B.I. do responsável.

## Regra de negócio proposta
Para estudante vinculado a academia de nível `escola`:
- `bilhete_identidade_responsavel` é obrigatório.
- `telefone_responsavel` é obrigatório.
- `bilhete_identidade` do estudante deve ser obrigatório quando a idade/ano académico indicar que o estudante já deve possuir B.I.; caso contrário, permitir cédula/documento alternativo.
- B.I. do estudante e B.I. do responsável não podem ser iguais.
- Telefone do responsável deve ser normalizado e validado.

Para academia de nível `superior`:
- Responsável pode ser opcional, salvo regra de matrícula específica.
- B.I. do próprio estudante deve ser obrigatório.

## Modelo de dados sugerido
Adicionar em `projection_estudantes` e nos eventos de estudante:
- `telefone_responsavel`.
- `nome_responsavel`, opcional.
- `parentesco_responsavel`, opcional.

Adicionar em solicitações de matrícula:
- `telefone_responsavel`.
- `nome_responsavel`, opcional.
- `parentesco_responsavel`, opcional.

## Fluxo operacional: cadastro direto pela academia
1. Academia envia dados do estudante.
2. Sistema carrega academia autenticada.
3. Se academia for `escola`, sistema exige `bilhete_identidade_responsavel` e `telefone_responsavel`.
4. Sistema valida formato e unicidade do B.I. do estudante, quando informado.
5. Sistema valida que B.I. do estudante e do responsável são diferentes.
6. Sistema grava evento com dados do responsável.
7. Projeção passa a expor dados do responsável para listagens e detalhes permitidos.

## Fluxo operacional: solicitação de matrícula
1. Responsável preenche solicitação.
2. Sistema exige B.I. do responsável e PDF do B.I. do responsável.
3. Sistema exige telefone do responsável.
4. Sistema exige documento do estudante conforme regra de idade/nível.
5. Academia analisa solicitação.
6. Ao aprovar, os dados do responsável são copiados para o cadastro do estudante.

## Validações
- `telefone_responsavel` não pode ser vazio para nível escolar.
- Telefone deve aceitar padrão local configurável, mas rejeitar texto inválido.
- B.I. do responsável deve ter tamanho/formato mínimo configurável.
- B.I. do estudante não pode duplicar outro estudante, quando informado.
- B.I. do estudante não pode ser igual ao B.I. do responsável.
- Atualizações de dados pessoais devem preservar a obrigatoriedade.

## Critérios de aceite
- Cadastro escolar sem B.I. do responsável é rejeitado.
- Cadastro escolar sem telefone do responsável é rejeitado.
- Solicitação de matrícula escolar sem telefone do responsável é rejeitada.
- Dados antigos sem telefone do responsável podem ser migrados com status `pendente_complemento` em vez de bloqueio imediato.
- Testes cobrem cadastro direto, solicitação pública e atualização de dados pessoais.
