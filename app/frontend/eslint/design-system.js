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

const isMotionElement = (name) => name.type === 'JSXMemberExpression' && name.object.name === 'motion';

const noRawElements = {
    meta: { type: 'problem', schema: [] },
    create(context) {
        return {
            JSXOpeningElement(node) {
                const use = isMotionElement(node.name)
                    ? '<MotionFrame> from @/design/motion'
                    : Object.hasOwn(REPLACEMENTS, node.name.name) && REPLACEMENTS[node.name.name];
                if (!use) return;
                context.report({
                    node,
                    message: `Use ${use} instead of <${context.sourceCode.getText(node.name)}>: raw elements accept any class string, and the design system only works when there is no way around it.`,
                });
            },
        };
    },
};

const noStyleProps = {
    meta: { type: 'problem', schema: [] },
    create(context) {
        return {
            JSXAttribute(node) {
                if (node.name.name !== 'className' && node.name.name !== 'style') return;
                context.report({
                    node,
                    message: `Express the look through Frame and Text props or a component variant instead of ${node.name.name}. When no token fits, add one to src/design/tokens.ts or a variant to the component in src/components/ui/.`,
                });
            },
        };
    },
};

export default {
    meta: { name: 'design-system' },
    rules: { 'no-raw-elements': noRawElements, 'no-style-props': noStyleProps },
};
