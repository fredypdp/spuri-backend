---
modificado: 2026-06-21 19:07
criado: 2026-05-01 14:02
---
1. Separar o ano letivo escolar e superior (`type: escolar/superior`) e permitir a definição do inicio e fim de cada um. O sistema (apenas admin FPP pode fazer isso) deve definir o mês de começo e término (`periodo: [mês inicial]_[mês final]`, o mês é em número de 1-12, exemplo: `periodo: "09_07"`) do ano letivo de cada tipo (isso será uma constante), enquanto que o ano letivo mudará ao longo do tempo. As datas das faltas devem estar dentro desse limite.
	1. Exemplo:
	      `ano_letivo: "2025_2026",` - Isso muda ao longo do tempo
	      `type: "escolar",` - Esse valor pode ser editado, mas nunca muda ao longo do tempo nem quando o `ano_letivo` muda ,mas não podem existir dois ano letivo com o mesmo type
	      `periodo: "10_07"` - Esse valor pode ser editado, mas nunca muda ao longo do tempo nem quando o `ano_letivo` muda.
	    Obs: O mês de inicio é um mês do ano de início do ano letivo, e o mês do final pertence ao ano que o ano letivo termina. Nesse exemplo, o ano letivo começa em outubro de 2025 e termina em julho de 2026.
2. Permitir que as academias definam que o ano letivo delas terminou, isso vai ajudar a mitigar a má definição do ano letivo por parte da plataforma, se todas as academias definiram que um determinado ano letivo foi finalizado, a plataforma não pode definir um ano letivo anterior a esse, deve ser sempre o seguinte.
3. Testar a validação de integridade dos dados (event sourcing), ou seja, se o sistema (tanto o código GO e outros, quanto o banco de dados) realmente impede a adulteração dos dados e se os rebuilds estão funcionando corretamente
4. Adicionar sumários/aulas (para usar nas faltas (nas faltas adicionar campo opcional "sumario_id" e "sumario_titulo"), contagem trimestral/semestral dos sumários de ano acadêmico, e estatísticas futuras etc...). Exemplo de estrutura: `{sumario_titulo, periodo: trimestre/semestre, ano_academico, nivel: escolar/superior, type: medio/superior, curso_id (se for do médio ou superior), semestre (caso seja do nível do "superior")}`. Deve ter validações de segurança como o "nivel" ser preenchido automaticamente pelo sistema dependendo da academia que requisitou, além dos outros campo terem proteções para serem preenchidos com valores que estejam de acordo com o escopo/contexto da que requisitou.
5. Permitir às academias adicionar/remover anos acadêmicos (com as devidas validações de segurança avançadas)