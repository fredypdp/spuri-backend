# Depurar obrigatoriedade de email e telefone no cadastro de administradores

## Verificações executadas

```bash
rg -n "RegisterAdmin|AdminCriado|telefone|email" internal docs Documentação.md
```

## Resultado

A auditoria confirmou que o cadastro autenticado de administradores (`POST /dominis/register`) exigia `nome`, `email` e `role`, mas não exigia `telefone`. O aggregate `Admin.Criar` também validava email, senha e role, porém não recebia nem persistia o telefone no evento `AdminCriado`.

## Correções aplicadas

1. `POST /dominis/register` passa a exigir `telefone` no payload, normalizar o número e validar o formato nativo de 9 dígitos antes de criar qualquer administrador.
2. `Admin.Criar` passa a exigir `email` e `telefone` para todas as roles (`fpp`, `adm` e `gerente`).
3. O evento `AdminCriado` passa a carregar `telefone`, e a projeção `projection_admins` passa a preencher a coluna `telefone` ao processar o evento.
4. O bootstrap do primeiro admin FPP também passa a exigir telefone, para manter a regra de obrigatoriedade em qualquer cadastro de administrador.
5. A documentação principal foi atualizada para marcar `telefone` como obrigatório no cadastro de administradores.

## Testes adicionados

- `TestAdminCriarExigeEmailETelefoneParaQualquerRole`
- `TestAdminCriarPersisteTelefoneNoEventoEEstado`
- `TestAdminCriarValidaFormatoTelefone`
