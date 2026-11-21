// Rules that keep markup on the design system: containers and text go through Frame and Text, controls through
// Button and Link, and only the primitives in src/components/ui/ and src/design/ write class strings.

const FRAME = [
    'div',
    'section',
    'nav',
    'header',
    'footer',
    'main',
    'aside',
    'article',
    'ul',
    'ol',
    'li',
    'form',
    'fieldset',
];
const TEXT = ['p', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'code', 'strong', 'em', 'time', 'small', 'dt', 'dd'];

const REPLACEMENTS = {
    ...Object.fromEntries(FRAME.map((name) => [name, `<Frame as="${name}">`])),
    ...Object.fromEntries(TEXT.map((name) => [name, `<Text as="${name}">`])),
    div: '<Frame>',
    p: '<Text>',
    span: '<Text as="span"> for text or <Frame as="span"> for a row inside a phrasing parent',
    label: '<Text as="label"> or <Frame as="label">',
    button: '<Button> from @/components/ui/button, adding a variant when none fits',
    a: '<Link> from @/components/ui/link, adding a variant when none fits',
};

const MOTION_SOURCES = new Set(['motion/react', 'motion/react-m', 'framer-motion']);
const MOTION_EXPORTS = new Set(['motion', 'm']);

const rawElementMessage = (use, element) =>
    `Use ${use} instead of ${element}: raw elements accept any class string, and the design system only works when there is no way around it.`;

// The local names an import gives some exports, whether imported by name, under an alias, or through a namespace.
function trackImports(sources, exports, names = []) {
    const local = new Set(names);
    const namespaces = new Set();
    const collect = (node) => {
        if (!sources.has(node.source.value)) return;
        for (const specifier of node.specifiers) {
            if (specifier.type !== 'ImportSpecifier') namespaces.add(specifier.local.name);
            else if (exports.has(specifier.imported.name)) local.add(specifier.local.name);
        }
    };
    const isIdentifier = (node) =>
        (node.type === 'Identifier' || node.type === 'JSXIdentifier') && local.has(node.name);
    const isMember = (node) =>
        (node.type === 'MemberExpression' || node.type === 'JSXMemberExpression') &&
        namespaces.has(node.object.name) &&
        exports.has(node.property.name);
    return { collect, refersTo: (node) => isIdentifier(node) || isMember(node) };
}

const rawName = (node) =>
    node?.type === 'Literal' && typeof node.value === 'string' && Object.hasOwn(REPLACEMENTS, node.value) && node.value;

const noRawElements = {
    meta: { type: 'problem', schema: [] },
    create(context) {
        const motion = trackImports(MOTION_SOURCES, MOTION_EXPORTS, ['motion']);
        // React's createElement, not the DOM's document.createElement.
        const react = trackImports(new Set(['react']), new Set(['createElement']));
        const isMotionFactory = (callee) =>
            motion.refersTo(callee) ||
            (callee.type === 'MemberExpression' && callee.property.name === 'create' && motion.refersTo(callee.object));
        const elementUse = (name) =>
            name.type === 'JSXMemberExpression' && motion.refersTo(name.object)
                ? '<MotionFrame> from @/design/motion'
                : Object.hasOwn(REPLACEMENTS, name.name) && REPLACEMENTS[name.name];
        return {
            ImportDeclaration(node) {
                motion.collect(node);
                react.collect(node);
            },
            JSXOpeningElement(node) {
                const use = elementUse(node.name);
                if (use)
                    context.report({
                        node,
                        message: rawElementMessage(use, `<${context.sourceCode.getText(node.name)}>`),
                    });
            },
            // createElement('div'), motion('div') and motion.create('div') build the same raw elements without JSX.
            CallExpression(node) {
                const element = rawName(node.arguments[0]);
                if (!element) return;
                if (isMotionFactory(node.callee))
                    context.report({
                        node,
                        message: rawElementMessage('<MotionFrame> from @/design/motion', `motion('${element}')`),
                    });
                else if (react.refersTo(node.callee))
                    context.report({
                        node,
                        message: rawElementMessage(REPLACEMENTS[element], `createElement('${element}')`),
                    });
            },
        };
    },
};

const STYLE_PROPS = new Set(['className', 'style']);

const stylePropMessage = (name) =>
    `Express the look through Frame and Text props or a component variant instead of ${name}. When no token fits, add one to src/design/tokens.ts or a variant to the component in src/components/ui/.`;

const noStyleProps = {
    meta: { type: 'problem', schema: [] },
    create(context) {
        return {
            JSXAttribute(node) {
                if (STYLE_PROPS.has(node.name.name))
                    context.report({ node, message: stylePropMessage(node.name.name) });
            },
            // Props written as an object, such as a spread ({...{ className }}), createElement's props or cloneElement's overrides.
            Property(node) {
                if (node.parent.type !== 'ObjectExpression' || node.computed) return;
                const name = node.key.type === 'Identifier' ? node.key.name : node.key.value;
                if (STYLE_PROPS.has(name)) context.report({ node, message: stylePropMessage(name) });
            },
        };
    },
};

const PALETTE =
    'slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose';
const COLOR_UTILITY =
    '(bg|text|border(-[xytrblse])?|ring|ring-offset|inset-ring|shadow|inset-shadow|outline|decoration|divide|fill|stroke|caret|accent|placeholder|from|via|to|btn-surface)';

const TOKEN_MESSAGES = {
    dark: 'Give the token both theme values in src/index.css instead of a dark: variant: the .dark class swaps every variable, so no component needs a second styling pass.',
    palette:
        'Use a semantic color token from src/index.css, not a raw palette color: a palette value carries no decision about what it is for.',
    opacity:
        'Name the translucent color as its own token in src/index.css instead of an opacity modifier, so it has a value in both themes.',
    value: 'Add a token instead of an arbitrary value: an arbitrary value is a decision no one else can reuse.',
    property:
        'Add a token or an @utility in src/index.css instead of an arbitrary property: an arbitrary property is a decision no one else can reuse.',
};

const PALETTE_COLOR = new RegExp(`^${COLOR_UTILITY}-((${PALETTE})-\\d{2,3}|white|black)(/.*)?$`);
// A modifier after a color is its opacity; after a text size it is the line height, which the spacing scale already decides.
const OPACITY_MODIFIER = new RegExp(
    `^(?!text-(xs|sm|base|lg|\\d?xl)/)(?!bg-(linear|radial|conic)-)${COLOR_UTILITY}-[a-z0-9-]+/`,
);
// transition-[...] lists properties, which the transition rule checks.
const ARBITRARY_VALUE = /^(?!transition-)[a-z@][a-z0-9-]*-\[|\/\[/;
const ARBITRARY_PROPERTY = /^\[-*[a-z][a-z-]*:/;
// Bracketed variants select an element by state, such as data-[side=top]:; breakpoints are values.
const ARBITRARY_BREAKPOINT = /^@?(min|max)-\[|^@\[/;

// Splits hover:data-[side=top]:bg-x into its variants and utility, keeping colons inside brackets and parentheses.
function classParts(cls) {
    const parts = [];
    let depth = 0;
    let start = 0;
    for (let i = 0; i < cls.length; i++) {
        if ('[('.includes(cls[i])) depth++;
        else if ('])'.includes(cls[i])) depth--;
        else if (cls[i] === ':' && depth === 0) {
            parts.push(cls.slice(start, i));
            start = i + 1;
        }
    }
    const utility = cls.slice(start).replace(/^!|!$/g, '').replace(/^-/, '');
    return { variants: parts, utility };
}

function tokenProblem(cls, allowDark) {
    const { variants, utility } = classParts(cls);
    if (!allowDark && variants.includes('dark')) return 'dark';
    if (variants.some((variant) => ARBITRARY_BREAKPOINT.test(variant))) return 'value';
    if (ARBITRARY_PROPERTY.test(utility)) return 'property';
    if (ARBITRARY_VALUE.test(utility)) return 'value';
    if (PALETTE_COLOR.test(utility)) return 'palette';
    if (OPACITY_MODIFIER.test(utility)) return 'opacity';
    return null;
}

// Every string is read as a class list: class names reach components through variables, cva maps and helpers, not only className.
const tokenClasses = {
    meta: {
        type: 'problem',
        schema: [{ type: 'object', properties: { allowDark: { type: 'boolean' } }, additionalProperties: false }],
    },
    create(context) {
        const allowDark = context.options[0]?.allowDark ?? false;
        const check = (node, text) => {
            for (const cls of text.split(/\s+/)) {
                const problem = cls && tokenProblem(cls, allowDark);
                if (problem) context.report({ node, message: `${cls}: ${TOKEN_MESSAGES[problem]}` });
            }
        };
        return {
            Literal(node) {
                if (typeof node.value === 'string') check(node, node.value);
            },
            TemplateElement(node) {
                check(node, node.value.raw);
            },
        };
    },
};

export default {
    meta: { name: 'design-system' },
    rules: { 'no-raw-elements': noRawElements, 'no-style-props': noStyleProps, 'token-classes': tokenClasses },
};
