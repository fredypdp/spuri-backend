---
modificado: 2026-06-28 3:03
criado: 2026-06-15 18:06
---
# Cadastro de estudante escolar: BI do responsável obrigatório

## Objetivo

Implementar e consolidar as regras de validação de Bilhete de Identidade (BI) para estudantes do nível escolar, garantindo que todo estudante escolar tenha BI do responsável informado e que não existam conflitos entre o BI do estudante, o BI do responsável e o BI de outros estudantes escolares.

> Observação: a regra relacionada ao telefone do responsável não faz parte desta tarefa, pois já foi implementada.

## Estado atual observado

Na solicitação de matrícula, o backend exige `bilhete_identidade_responsavel` textual e PDF `bi_responsavel`. Também exige `bi_estudante` em PDF ou `cedula_estudante` quando não houver BI do estudante. Na aprovação da solicitação, o estudante é criado a partir dos dados da solicitação.

A migração de unicidade do BI principal indica que `bilhete_identidade` do estudante é único quando preenchido, mas não torna o campo obrigatório no banco para todos os estudantes.

## Regra de negócio a implementar

### Para estudantes do nível escolar/fundamental/médio

- `bilhete_identidade_responsavel` deve ser obrigatório.
- O documento PDF `bi_responsavel` deve ser obrigatório quando o fluxo exigir anexos de documentação.
- O BI do estudante, quando informado, não pode ser igual ao `bilhete_identidade_responsavel` do próprio estudante.
- O BI do estudante não pode ser igual ao BI de outro estudante que também não esteja no ensino superior.
- A validação de duplicidade deve considerar apenas estudantes não superiores para esta regra específica, ou seja, estudantes escolares/fundamental/médio.
- O BI do responsável pode repetir entre estudantes diferentes, por exemplo, no caso de irmãos com o mesmo responsável.
- Não aceitar valores vazios ou compostos apenas por espaços após `trim`.

### Para estudantes do ensino superior

- Esta tarefa não altera as regras de BI de estudantes do ensino superior.
- A restrição nova de “BI do estudante não pode ser igual ao BI de outro estudante” deve ser aplicada somente contra estudantes que não estejam no ensino superior, conforme a regra solicitada.

## Ajuste necessário

Aplicar as validações em todos os fluxos que criam ou aprovam estudantes escolares:

- Solicitação de matrícula de estudante escolar.
- Aprovação de solicitação de matrícula que cria o estudante.
- Cadastro direto de estudante pela academia.
- Atualização de dados cadastrais, caso o endpoint permita alterar BI do estudante ou BI do responsável.

As validações devem ser feitas antes de persistir o estudante ou aprovar a solicitação, retornando erro claro quando a regra for violada.

## Validações obrigatórias

- Estudante escolar sem `bilhete_identidade_responsavel`: deve falhar.
- Estudante escolar com `bilhete_identidade_responsavel` vazio após `trim`: deve falhar.
- Estudante escolar com BI do estudante igual ao BI do responsável: deve falhar.
- Estudante escolar com BI do estudante igual ao BI de outro estudante não superior: deve falhar.
- Estudante escolar com BI do responsável igual ao BI do responsável de outro estudante: deve passar.
- Cadastro direto pela academia deve aplicar as mesmas regras da solicitação de matrícula.
- Aprovação de solicitação de matrícula deve revalidar as regras antes de criar o estudante, para evitar inconsistências caso os dados tenham sido alterados ou fiquem obsoletos.

## Fluxo operacional na solicitação de matrícula

1. Solicitante informa dados do estudante.
2. Informa `bilhete_identidade_responsavel`.
3. Envia PDF do BI do responsável quando exigido pelo fluxo documental.
4. Informa BI do estudante ou documento alternativo quando a política do produto permitir estudante escolar sem BI próprio.
5. Backend normaliza os BIs usados na comparação, por exemplo aplicando `trim` e o mesmo padrão de comparação já usado no sistema.
6. Backend valida que o BI do estudante não é igual ao BI do responsável.
7. Backend valida que o BI do estudante não é igual ao BI de outro estudante não superior.
8. Solicitação fica pendente somente se todas as regras forem atendidas.
9. Ao aprovar, backend reexecuta as validações e cria o estudante com os mesmos dados validados.

## Fluxo operacional no cadastro direto

1. Academia chama endpoint de cadastro de estudante.
2. Backend identifica se o estudante pertence ao nível escolar/fundamental/médio ou ao ensino superior com base nas regras existentes do domínio.
3. Se for estudante escolar, exige `bilhete_identidade_responsavel`.
4. Se houver BI do estudante, backend valida que ele é diferente do BI do responsável.
5. Backend valida que o BI do estudante não pertence a outro estudante não superior.
6. Backend grava evento `EstudanteCriadoComVinculo` somente após validação.

## Testes recomendados

- Solicitação escolar sem `bilhete_identidade_responsavel`: deve falhar.
- Solicitação escolar com `bilhete_identidade_responsavel` contendo apenas espaços: deve falhar.
- Solicitação escolar sem PDF `bi_responsavel`, quando o fluxo documental exigir anexo: deve falhar.
- Solicitação escolar com BI do estudante igual ao BI do responsável: deve falhar.
- Solicitação escolar com BI do estudante igual ao BI de outro estudante escolar/fundamental/médio: deve falhar.
- Solicitação escolar com BI do estudante igual ao BI de estudante do ensino superior: validar conforme regra global existente de unicidade; esta tarefa não deve criar nova restrição específica para superior.
- Solicitação escolar com BI do responsável repetido entre irmãos: deve passar.
- Cadastro direto escolar sem `bilhete_identidade_responsavel`: deve falhar.
- Cadastro direto escolar com BI do estudante igual ao BI do responsável: deve falhar.
- Cadastro direto escolar com BI do estudante igual ao BI de outro estudante não superior: deve falhar.
- Aprovação de solicitação deve falhar se, entre a criação e a aprovação, outro estudante não superior tiver sido criado com o mesmo BI do estudante.
