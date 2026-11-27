const shapes = {
    calm: 'state-symbol-calm',
    streams: 'state-symbol-streams',
    shelf: 'state-symbol-shelf',
    loop: 'state-symbol-loop',
};

type StateSymbolShape = keyof typeof shapes;

function StateSymbol({ shape }: { shape: StateSymbolShape }) {
    return (
        <span aria-hidden="true" data-slot="state-symbol" className={`state-symbol block shrink-0 ${shapes[shape]}`} />
    );
}

export { StateSymbol, type StateSymbolShape };
