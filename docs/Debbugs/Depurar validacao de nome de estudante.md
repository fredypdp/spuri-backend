# Depurar validação de nome de estudante

## Objetivo

Garantir que o nome do estudante aceite apenas letras Unicode, acentos/sinais diacríticos comuns em nomes, espaços e apóstrofos, bloqueando números e sinais especiais/de pontuação como `@`, `#`, `$`, `?`, `!`, `_`, `-`, vírgulas e pontos.

## Resultado

- O cadastro de estudante passa a usar uma validação específica para nomes de estudantes no fluxo comum de matrícula.
- A solicitação de edição do campo `nome` reutiliza a mesma validação antes de criar a solicitação e antes de aplicar uma aprovação.
- O aggregate de estudante também valida o nome ao aplicar alteração aprovada por solicitação, mantendo a regra no domínio.
- Foram adicionados testes unitários cobrindo nomes com acentos e rejeição de números, sinais especiais e pontuação.
