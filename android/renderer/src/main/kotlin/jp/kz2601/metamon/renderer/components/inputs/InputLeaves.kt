package jp.kz2601.metamon.renderer.components.inputs

import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.renderer.registry.ComponentRegistry

/** 入力の Leaf 12 種(#858)。1 行 1 部品・名前順。 */
fun ComponentRegistry.Builder.registerInputLeaves() {
    register(ComponentType.COLOR_PICKER) { c, ctx, m -> ColorPickerLeaf(c, ctx, m) }
    register(ComponentType.DATE_PICKER) { c, ctx, m -> DatePickerLeaf(c, ctx, m) }
    register(ComponentType.NUMBER_FIELD) { c, ctx, m -> NumberFieldLeaf(c, ctx, m) }
    register(ComponentType.PICKER) { c, ctx, m -> PickerLeaf(c, ctx, m) }
    register(ComponentType.RATING) { c, ctx, m -> RatingLeaf(c, ctx, m) }
    register(ComponentType.SECURE_FIELD) { c, ctx, m -> SecureFieldLeaf(c, ctx, m) }
    register(ComponentType.SLIDER) { c, ctx, m -> SliderLeaf(c, ctx, m) }
    register(ComponentType.STEPPER) { c, ctx, m -> StepperLeaf(c, ctx, m) }
    register(ComponentType.TEXT_EDITOR) { c, ctx, m -> TextEditorLeaf(c, ctx, m) }
    register(ComponentType.TEXT_FIELD) { c, ctx, m -> TextFieldLeaf(c, ctx, m) }
    register(ComponentType.TIME_PICKER) { c, ctx, m -> TimePickerLeaf(c, ctx, m) }
    register(ComponentType.TOGGLE) { c, ctx, m -> ToggleLeaf(c, ctx, m) }
}
