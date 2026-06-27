---
modificado: 2026-06-27 23:11
criado: 2026-06-14 19:07
---
**1. Remover completamente o conceito de Telefone Extra**, e sem deixar resquício desse código legado
**2. Adicionar campos de telefone nativos às entidades:**

Para o Estudante:
- `telefone` — string
- `telefone_verificado` — boolean (default `false`)
- `telefone_responsavel` — string (opcional para estudante do ensino superior)
- `telefone_responsavel_verificado` — boolean (default `false`)
Nota: `telefone` e `telefone_responsavel` não podem estar vazios ao mesmo tempo, pelo menos um deles deve ser preenchido
Para a Academia , o campo `numero_telefone` já existe — renomear para `telefone` para consistência com as demais entidades, e adicionar:
- `telefone_verificado` — boolean (default `false`)

Para o Admin:
- `telefone`
- `telefone_verificado` — boolean (default `false`)

**3. Regra de negócio importante a documentar:** A verificação de número de telefone **ainda não está implementada**. Os campos `telefone_verificado`, `telefone_responsavel_verificado` existem na estrutura de dados mas **nenhum fluxo de verificação está ativo**. Os campos são mantidos para evitar conflitos futuros na base de dados quando a funcionalidade for implementada. Nenhum endpoint de verificação de telefone deve ser documentado por enquanto. Além disso não pode ser adicionado um número que já foi verificado por outro usuário, mas se ainda não foi, pode ser.

**4. Normalização de telefone:** Manter a mesma lógica de normalização que existia nos telefones extras: remove espaços, hifens e parênteses, e o telefone só tem 9 carácteres numéricos (mas salvar como string) e não tem sem ddi. Aplicar essa regra agora nos campos nativos de todas as entidades. E para o estudante, o `telefone_responsavel` não pode ser igual ao `telefone` dele mesmo que nenhum esteja verificado, além de que o `telefone` de um estudante não pode ser o `telefone_responsavel` de outro

**5. Atualizar as documentações (e suas versões) e tudo que foi afetado**