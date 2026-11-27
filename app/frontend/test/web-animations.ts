// happy-dom's Animation.cancel() leaves its finished promise rejected and unhandled:
// https://github.com/capricorn86/happy-dom/issues/2412. Motion only takes Web Animations when Element.prototype
// has its own animate (https://github.com/motiondivision/motion/blob/main/packages/motion-dom/src/animation/waapi/supports/waapi.ts),
// so without it motion runs its own frame loop.
export function animateWithoutWebAnimations() {
    Reflect.deleteProperty(Element.prototype, 'animate');
}
