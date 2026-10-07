// AxisNow 设计 token + Naive UI themeOverrides（从 lansonsam.axisnow.io 线上 bundle 提取）
import type { GlobalThemeOverrides } from "naive-ui"

export const sizeTokens = {
  "card-border-radius": "10px",
  "button-border-radius": "6px",
  "text-font-size-sm": "12px",
  "text-font-size-md": "14px",
  "text-font-size-lg": "16px",
  "text-font-size-xl": "18px",
  "text-font-size-2xl": "20px",
  "text-font-size-3xl": "22px",
  "text-font-size-4xl": "24px",
  "text-font-size-5xl": "26px",
  "text-font-size-6xl": "28px",
  "text-font-size-7xl": "30px",
  "text-font-size-8xl": "32px",
  "text-font-size-9xl": "34px",
  "text-font-size-10xl": "36px",
  "spacing-xs": "6px",
  "spacing-sm": "10px",
  "spacing-md": "20px",
  "spacing-lg": "30px"
} as const

export const lightTokens = {
  "color-body": "#ffffff",
  "color-primary": "#ffffff",
  "color-secondary": "#fafbfc",
  "color-tertiary": "#fafbfc",
  "color-quaternary": "#fafbfc",
  "color-fill": "#f4f5f6",
  "color-disabled": "#e2e4e8",
  "color-selected": "#f5f5f5",
  "color-action": "#18181b",
  "color-action-hover": "rgba(0, 0, 0, 0.06)",
  "color-action-pressed": "rgba(0, 0, 0, 0.12)",
  "color-action-selection": "rgba(24, 24, 27, 0.12)",
  "color-inverse": "#171a1f",
  "color-overlay": "rgba(0, 0, 0, 0.4)",
  "color-border-default": "rgba(31, 35, 40, 0.14)",
  "color-border-strong": "rgba(31, 35, 40, 0.26)",
  "color-text-on-action": "#fafafa",
  "color-text-primary": "#18181b",
  "color-text-secondary": "rgba(31, 35, 40, 0.68)",
  "color-text-tertiary": "rgba(31, 35, 40, 0.5)",
  "color-text-quaternary": "rgba(31, 35, 40, 0.36)",
  "color-text-default": "#3f444a",
  "color-text-disabled": "rgba(31, 35, 40, 0.3)",
  "color-text-info": "#007aff",
  "color-text-error": "#d92d20",
  "color-text-critical": "#b42318",
  "color-text-warning": "#b7791f",
  "color-text-success": "#2f855a",
  "color-text-keyword": "#6f63d9",
  "color-text-inverse": "#ffffff",
  "color-table-row-hover": "#f5f5f5",
  "color-table-header": "#fafbfc",
  "color-switch-rail": "#d6d6d6",
  "color-map-water": "#ffffff",
  "color-map-land": "#d6d6d6",
  "color-code-surface": "#ffffff",
  "color-code-key": "#6f42c1",
  "color-code-string": "#1a7f37",
  "color-code-number": "#0550ae",
  "color-code-boolean": "#0550ae",
  "color-palette-1": "#2f7fed",
  "color-palette-2": "#00bbad",
  "color-palette-3": "#5954d6",
  "color-palette-4": "#7cb342",
  "color-palette-5": "#d163e6",
  "color-palette-6": "#b80058",
  "color-palette-7": "#ff9287",
  "color-palette-8": "#b24502",
  "color-palette-9": "#ebac23",
  "color-palette-10": "#0eaa42",
  "color-semantics-gray": "#6a7281",
  "color-semantics-red": "#de221b",
  "color-semantics-orange": "#b85417",
  "color-semantics-yellow": "#906c12",
  "color-semantics-green": "#10842b",
  "color-semantics-teal": "#108078",
  "color-semantics-blue": "#1c6de0",
  "color-semantics-indigo": "#625eeb",
  "color-semantics-purple": "#bb1bde",
  "color-semantics-pink": "#d71b76"
} as const

export const darkTokens = {
  "color-body": "#111316",
  "color-primary": "#1a1d23",
  "color-secondary": "#1e2128",
  "color-tertiary": "#262930",
  "color-quaternary": "#363a46",
  "color-fill": "#22252c",
  "color-disabled": "#2a2e37",
  "color-selected": "#2f343e",
  "color-action": "#fafafa",
  "color-action-hover": "rgba(255, 255, 255, 0.08)",
  "color-action-pressed": "rgba(255, 255, 255, 0.14)",
  "color-action-selection": "rgba(250, 250, 250, 0.2)",
  "color-inverse": "#fff",
  "color-overlay": "rgba(0, 0, 0, 0.6)",
  "color-border-default": "rgba(255, 255, 255, 0.08)",
  "color-border-strong": "rgba(255, 255, 255, 0.16)",
  "color-text-on-action": "#18181b",
  "color-text-primary": "#fafafa",
  "color-text-secondary": "rgba(235, 235, 245, 0.6)",
  "color-text-tertiary": "rgba(235, 235, 245, 0.3)",
  "color-text-quaternary": "rgba(235, 235, 245, 0.18)",
  "color-text-default": "#ebebf5",
  "color-text-disabled": "rgba(235, 235, 245, 0.3)",
  "color-text-info": "#0a84ff",
  "color-text-error": "#ff453a",
  "color-text-critical": "#ff453a",
  "color-text-warning": "#ffd60a",
  "color-text-success": "#30d158",
  "color-text-keyword": "#b6abff",
  "color-text-inverse": "#171a1f",
  "color-table-row-hover": "#242832",
  "color-table-header": "#242832",
  "color-switch-rail": "#363a46",
  "color-map-water": "#1e2128",
  "color-map-land": "#363a46",
  "color-code-surface": "#262930",
  "color-code-key": "#d19a66",
  "color-code-string": "#98c379",
  "color-code-number": "#d19a66",
  "color-code-boolean": "#56b6c2",
  "color-palette-1": "#5a9eff",
  "color-palette-2": "#22d3c5",
  "color-palette-3": "#8b86ff",
  "color-palette-4": "#a3e635",
  "color-palette-5": "#e889f4",
  "color-palette-6": "#ff5c9f",
  "color-palette-7": "#ff9f97",
  "color-palette-8": "#d9893d",
  "color-palette-9": "#f7c948",
  "color-palette-10": "#39d972",
  "color-semantics-gray": "#aab3c0",
  "color-semantics-red": "#ff8a80",
  "color-semantics-orange": "#e69f5c",
  "color-semantics-yellow": "#f7c948",
  "color-semantics-green": "#39d972",
  "color-semantics-teal": "#22d3c5",
  "color-semantics-blue": "#7cb0ff",
  "color-semantics-indigo": "#a5a1ff",
  "color-semantics-purple": "#e889f4",
  "color-semantics-pink": "#ff8ab8"
} as const

/** ECharts 用色（palette 为系列色顺序） */
export const chartColors = {
  "light": {
    "default": "#6b7280",
    "info": "#007aff",
    "success": "#2f855a",
    "successAccent": "#b7e4c7",
    "warning": "#b7791f",
    "error": "#d92d20",
    "critical": "#b42318",
    "disabled": "rgba(107, 114, 128, 0.45)",
    "palette": [
      "#2f7fed",
      "#00bbad",
      "#5954d6",
      "#29bbd4",
      "#d163e6",
      "#b80058",
      "#ff9287",
      "#b24502",
      "#ebac23",
      "#0eaa42"
    ]
  },
  "dark": {
    "default": "#ebebf5",
    "info": "#0a84ff",
    "success": "#30d158",
    "successAccent": "#acedbc",
    "warning": "#ffd60a",
    "error": "#ff453a",
    "critical": "#ff453a",
    "disabled": "rgba(235, 235, 245, 0.3)",
    "palette": [
      "#5a9eff",
      "#22d3c5",
      "#8b86ff",
      "#4fd8eb",
      "#e889f4",
      "#ff5c9f",
      "#ff9f97",
      "#d9893d",
      "#f7c948",
      "#39d972"
    ]
  }
} as const

type Tokens = Record<string, string>

function semantic(v: Tokens, dark: boolean) {
  const o: Record<string, string> = {colorBody:v["color-body"],
    colorPrimary:v["color-primary"],
    colorSecondary:v["color-secondary"],
    colorTertiary:v["color-tertiary"],
    colorQuaternary:v["color-quaternary"],
    colorFill:v["color-fill"],
    colorAction:v["color-action"],
    colorInverse:v["color-inverse"],
    colorOverlay:v["color-overlay"],
    colorBorderDefault:v["color-border-default"],
    colorBorderStrong:v["color-border-strong"],
    colorTextOnAction:v["color-text-on-action"],
    colorTextPrimary:v["color-text-primary"],
    colorTextSecondary:v["color-text-secondary"],
    colorTextTertiary:v["color-text-tertiary"],
    colorTextQuaternary:v["color-text-quaternary"],
    colorTextDisabled:v["color-text-disabled"],
    colorTextInfo:v["color-text-info"],
    colorTextWarning:v["color-text-warning"],
    colorTextError:v["color-text-error"],
    colorTextCritical:v["color-text-critical"],
    colorTextSuccess:v["color-text-success"],
    colorTextKeyword:v["color-text-keyword"],
    colorTextInverse:v["color-text-inverse"],
    tableRowHoverColor:v["color-table-row-hover"],
    tableHeaderColor:v["color-table-header"],
    tableBodyColor:v["color-body"],
    switchRailColor:v["color-switch-rail"],
    disabledColor:v["color-disabled"],
    cardColor:v["color-primary"],
    cardColorModal:v["color-primary"],
    cardBorderColor:v["color-border-default"],
    tagDefaultColor:v["color-fill"],
    actionSelectionColor:v["color-action-selection"],
    actionHoverColor:v["color-action-hover"]}
  if (dark) { o.tableBodyColor = v["color-primary"]; o.cardBorderColor = "transparent" }
  return o
}

function buildOverrides(t: ReturnType<typeof semantic>): GlobalThemeOverrides {
  return {common:{primaryColor:t.colorAction,primaryColorHover:t.colorAction,primaryColorPressed:t.colorAction,infoColor:t.colorTextInfo,bodyColor:t.colorBody,successColor:t.colorTextSuccess,warningColor:t.colorTextWarning,errorColor:t.colorTextError,textColor1:t.colorTextPrimary,textColor2:t.colorTextSecondary,textColor3:t.colorTextTertiary},Transfer:{listColor:`transparent`},Divider:{color:t.colorBorderDefault},Timeline:{lineColor:t.colorBorderDefault,iconColor:t.colorTextSecondary,titleTextColor:t.colorTextPrimary,contentTextColor:t.colorTextSecondary,metaTextColor:t.colorTextTertiary,circleBorder:`1px solid ${t.colorBorderStrong}`},Button:{color:`transparent`,colorHover:`transparent`,colorPressed:`transparent`,colorFocus:`transparent`,border:`1px solid ${t.colorBorderStrong}`,borderHover:`1px solid ${t.colorBorderStrong}`,borderPressed:`1px solid ${t.colorBorderStrong}`,borderFocus:`1px solid ${t.colorBorderStrong}`,textColor:t.colorTextPrimary,textColorHover:t.colorTextPrimary,textColorPressed:t.colorTextPrimary,textColorFocus:t.colorTextPrimary,colorSecondary:t.colorSecondary,colorPrimary:t.colorAction,colorHoverPrimary:t.colorAction,colorPressedPrimary:t.colorAction,colorFocusPrimary:t.colorAction,borderPrimary:`1px solid ${t.colorAction}`,borderHoverPrimary:`1px solid ${t.colorAction}`,borderPressedPrimary:`1px solid ${t.colorAction}`,borderFocusPrimary:`1px solid ${t.colorAction}`,rippleColorPrimary:t.colorAction,textColorPrimary:t.colorTextOnAction,textColorHoverPrimary:t.colorTextOnAction,textColorPressedPrimary:t.colorTextOnAction,textColorFocusPrimary:t.colorTextOnAction,colorError:t.colorTextError,borderError:`1px solid ${t.colorTextError}`,rippleColorError:t.colorTextError,textColorError:t.colorTextPrimary,colorSuccess:t.colorTextSuccess,borderSuccess:`1px solid ${t.colorTextSuccess}`,rippleColorSuccess:t.colorTextSuccess,textColorSuccess:t.colorPrimary,colorWarning:t.colorTextWarning,borderWarning:`1px solid ${t.colorTextWarning}`,rippleColorWarning:t.colorTextWarning,textColorWarning:t.colorPrimary,heightMedium:`36px`,borderRadiusMedium:`var(--button-border-radius)`,fontSizeMedium:`var(--text-font-size-md)`,heightLarge:`44px`,borderRadiusLarge:`var(--button-border-radius)`,fontSizeLarge:`var(--text-font-size-lg)`,heightSmall:`32px`,borderRadiusSmall:`var(--button-border-radius)`,fontSizeSmall:`var(--text-font-size-sm)`},DataTable:{paginationMargin:`10px 0`,tdColor:t.tableBodyColor,tdColorModal:t.tableBodyColor,thColor:t.tableHeaderColor,thColorModal:t.tableBodyColor,thTextColor:t.colorTextSecondary,tdTextColor:t.colorTextPrimary,thFontWeight:`bold`,tdColorHover:t.tableRowHoverColor,tdColorHoverModal:t.tableRowHoverColor,tdColorHoverPopover:t.tableRowHoverColor,borderColor:t.colorBorderDefault,borderColorModal:t.colorBorderDefault,borderRadius:`var(--card-border-radius)`,thIconColorActive:t.colorTextPrimary,thIconColor:t.colorTextPrimary,resizableSize:`1px`,loadingColor:t.colorTextSecondary},Pagination:{itemColorActive:t.colorAction,itemColorActiveHover:t.colorAction,itemTextColorActive:t.colorTextOnAction,itemTextColorActiveHover:t.colorTextOnAction,itemColorDisabled:t.colorTertiary,itemBorderRadius:`var(--card-border-radius)`},Card:{color:t.cardColor,colorModal:t.cardColorModal,borderColor:t.cardBorderColor,borderColorModal:t.cardBorderColor,borderRadius:`var(--card-border-radius)`,paddingMedium:`20px 30px`},Tag:{border:`1px solid ${t.colorBorderStrong}`,fontSizeMedium:`12px`,closeIconSizeMedium:`14px`,borderRadius:`8px`,padding:`0 10px`,color:t.tagDefaultColor,colorPrimary:t.colorAction,colorBordered:t.colorPrimary,colorBorderedPrimary:t.colorAction,borderPrimary:`1px solid ${t.colorAction}`,textColor:t.colorTextPrimary,textColorPrimary:t.colorTextOnAction,closeIconColorHover:t.colorTextPrimary,closeIconColorPressed:t.colorTextPrimary,closeIconColor:t.colorTextPrimary,closeColorHover:`transparent`,closeColorPressed:`transparent`,closeColorHoverPrimary:`transparent`,closeColorPressedPrimary:`transparent`,closeIconColorPrimary:t.colorTextOnAction,closeIconColorHoverPrimary:t.colorTextOnAction,closeIconColorPressedPrimary:t.colorTextOnAction,heightTiny:`18px`},Dropdown:{color:t.colorPrimary,borderRadius:`var(--card-border-radius)`,dividerColor:t.colorBorderDefault,optionTextColor:t.colorTextPrimary,optionTextColorHover:t.colorTextPrimary,optionTextColorActive:t.colorAction,optionTextColorChildActive:t.colorAction,prefixColor:t.colorTextPrimary,suffixColor:t.colorTextPrimary,optionColorHover:t.actionHoverColor,optionColorActive:t.actionHoverColor,groupHeaderTextColor:t.colorTextSecondary},Popover:{color:t.colorPrimary,borderRadius:`var(--card-border-radius)`},Popselect:{peers:{InternalSelectMenu:{color:t.colorPrimary,borderRadius:`var(--card-border-radius)`,optionTextColor:t.colorTextPrimary,optionTextColorActive:t.colorTextPrimary,optionTextColorPressed:t.colorTextPrimary,optionCheckColor:t.colorTextPrimary,optionColorPending:t.actionHoverColor,optionColorActive:`transparent`,optionColorActivePending:t.actionHoverColor}}},Tree:{nodeBorderRadius:`4px`,nodeColorHover:t.actionHoverColor,nodeColorPressed:t.actionSelectionColor,nodeColorActive:t.actionSelectionColor,nodeTextColor:t.colorTextPrimary},Tabs:{tabFontSizeMedium:`var(--text-font-size-md)`,closeIconSize:`var(--text-font-size-md)`,tabFontWeightActive:`bold`,tabBorderRadius:`6px`,tabPaddingMediumLine:`0px 0 8px`,colorSegment:t.colorPrimary,tabColorSegment:t.colorTextInverse,tabTextColorActiveSegment:t.colorTextPrimary},Input:{color:`transparent`,colorFocus:`transparent`,colorFocusError:`transparent`,colorFocusWarning:`transparent`,fontSizeMedium:`var(--text-font-size-md)`,borderRadius:`var(--card-border-radius)`,textColor:t.colorTextPrimary,textColorDisabled:t.colorTextSecondary,placeholderColor:t.colorTextSecondary,placeholderColorDisabled:t.colorTextTertiary,colorDisabled:t.disabledColor,border:`1px solid ${t.colorBorderStrong}`,borderHover:`1px solid ${t.colorAction}`,borderFocus:`1px solid ${t.colorBorderStrong}`,boxShadowFocus:`0 0 0 2px ${t.actionSelectionColor}`,borderDisabled:`1px solid ${t.colorBorderStrong}`,groupLabelColor:t.colorTertiary,groupLabelTextColor:t.colorTextPrimary,groupLabelBorder:`1px solid ${t.colorBorderStrong}`,borderError:`1px solid ${t.colorTextError}`,borderWarning:`1px solid ${t.colorTextWarning}`,borderSuccess:`1px solid ${t.colorTextSuccess}`},InternalSelection:{color:`transparent`,colorActive:`transparent`,colorActiveError:`transparent`,colorActiveWarning:`transparent`,fontSizeMedium:`var(--text-font-size-md)`,borderRadius:`var(--card-border-radius)`,textColor:t.colorTextPrimary,textColorDisabled:t.colorTextSecondary,placeholderColor:t.colorTextSecondary,placeholderColorDisabled:t.colorTextTertiary,colorDisabled:t.disabledColor,border:`1px solid ${t.colorBorderStrong}`,borderHover:`1px solid ${t.colorAction}`,borderFocus:`1px solid ${t.colorBorderStrong}`,borderActive:`1px solid ${t.colorBorderStrong}`,boxShadowHover:`none`,boxShadowFocus:`0 0 0 2px ${t.actionSelectionColor}`,boxShadowActive:`0 0 0 2px ${t.actionSelectionColor}`,borderDisabled:`1px solid ${t.colorBorderStrong}`,borderError:`1px solid ${t.colorTextError}`,borderWarning:`1px solid ${t.colorTextWarning}`,borderSuccess:`1px solid ${t.colorTextSuccess}`},Checkbox:{color:`transparent`,checkMarkColor:t.colorTextOnAction,border:`1px solid ${t.colorTextTertiary}`,borderDisabled:`1px solid ${t.colorBorderStrong}`,borderDisabledChecked:`1px solid ${t.colorBorderStrong}`,borderChecked:`1px solid ${t.colorAction}`,borderFocus:`1px solid ${t.colorTextTertiary}`,boxShadowFocus:`0 0 0 2px ${t.actionSelectionColor}`},Select:{peers:{InternalSelectMenu:{color:t.colorPrimary,borderRadius:`var(--card-border-radius)`,optionTextColor:t.colorTextPrimary,optionTextColorActive:t.colorTextPrimary,optionTextColorPressed:t.colorTextPrimary,optionCheckColor:t.colorTextPrimary,optionColorPending:t.actionHoverColor,optionColorActive:`transparent`,optionColorActivePending:t.actionHoverColor}}},AutoComplete:{peers:{InternalSelectMenu:{color:t.colorPrimary,borderRadius:`var(--card-border-radius)`,optionTextColor:t.colorTextPrimary,optionTextColorActive:t.colorTextPrimary,optionTextColorPressed:t.colorTextPrimary,optionCheckColor:t.colorTextPrimary,optionColorPending:t.actionHoverColor,optionColorActive:`transparent`,optionColorActivePending:t.actionHoverColor}}},Switch:{buttonColor:`#ffffff`,railColor:t.switchRailColor,railColorActive:t.colorTextSuccess},Icon:{color:t.colorTextPrimary},Radio:{boxShadow:`inset 0 0 0 1px ${t.colorBorderStrong}`,boxShadowActive:`inset 0 0 0 1px ${t.colorAction}`,boxShadowFocus:`inset 0 0 0 1px ${t.colorBorderStrong}, 0 0 0 2px ${t.actionSelectionColor}`,boxShadowHover:`inset 0 0 0 1px ${t.colorAction}`,boxShadowDisabled:`inset 0 0 0 1px ${t.colorBorderStrong}`,buttonColor:t.colorTertiary,buttonColorActive:t.colorAction,buttonBorderColor:t.colorBorderStrong,buttonBorderColorActive:t.colorAction,buttonBorderColorHover:t.colorAction,buttonTextColor:t.colorTextSecondary,buttonTextColorActive:t.colorTextOnAction,buttonBorderRadius:`6px`,buttonHeightMedium:`36px`,fontSizeMedium:`var(--text-font-size-md)`,buttonHeightLarge:`44px`,fontSizeLarge:`var(--text-font-size-lg)`,buttonHeightSmall:`32px`,fontSizeSmall:`var(--text-font-size-sm)`},Menu:{itemColorHover:t.colorQuaternary,itemColorActive:t.colorAction,itemColorActiveHover:t.colorAction,itemTextColor:t.colorTextPrimary,itemTextColorActive:t.colorTextOnAction,itemTextColorActiveHover:t.colorTextOnAction,itemIconColor:t.colorTextPrimary,itemIconColorActive:t.colorTextOnAction,itemIconColorActiveHover:t.colorTextOnAction},LoadingBar:{colorLoading:t.colorAction},Upload:{draggerColor:`transparent`,borderRadius:`var(--card-border-radius)`,draggerBorder:`1px dashed ${t.colorBorderStrong}`,draggerBorderHover:`1px dashed ${t.colorAction}`},Tooltip:{color:t.tagDefaultColor,textColor:t.colorTextSecondary,peers:{Popover:{fontSize:`var(--text-font-size-sm)`}}},TimePicker:{panelColor:t.colorTertiary,panelBorderRadius:`var(--card-border-radius)`},DatePicker:{panelColor:t.colorTertiary,panelBorderRadius:`var(--card-border-radius)`},Cascader:{menuColor:t.colorTertiary,menuBorderRadius:`var(--card-border-radius)`},TreeSelect:{menuColor:t.colorTertiary,menuBorderRadius:`var(--card-border-radius)`},Form:{labelFontWeight:`bold`,labelTextColor:t.colorTextSecondary,labelPaddingVertical:`0 0 0 0`,labelHeightMedium:`22px`},Skeleton:{borderRadius:`var(--card-border-radius)`},Spin:{color:t.colorTextSecondary,textColor:t.colorTextSecondary},Dialog:{color:t.colorPrimary,borderRadius:`12px`,padding:`24px`,iconSize:`22px`,closeMargin:`20px 20px 0 0`,titleFontSize:`18px`,titleFontWeight:`600`,titleTextColor:t.colorTextPrimary,textColor:t.colorTextSecondary,fontSize:`var(--text-font-size-md)`,contentMargin:`8px 0 20px 0`,actionSpace:`8px`,iconColorWarning:t.colorTextWarning,iconColorError:t.colorTextError,iconColorInfo:t.colorTextInfo,iconColorSuccess:t.colorTextSuccess}} as GlobalThemeOverrides
}

export const lightOverrides = buildOverrides(semantic(lightTokens, false))
export const darkOverrides = buildOverrides(semantic(darkTokens, true))

/** 注入 CSS 变量；主题切换用 <html data-theme="dark"> */
export function injectThemeVars() {
  const toVars = (o: Tokens) => Object.entries(o).map(([k, v]) => `--${k}: ${v};`).join(" ")
  let el = document.getElementById("ax-theme-vars")
  if (!el) { el = document.createElement("style"); el.id = "ax-theme-vars"; document.head.appendChild(el) }
  el.textContent = `:root { ${toVars({ ...sizeTokens, ...lightTokens })} color-scheme: light; } :root[data-theme="dark"] { ${toVars(darkTokens)} color-scheme: dark; }`
}

/** 读取 CSS 变量（给 ECharts 用，保证随主题变化） */
export const cssVar = (name: string) => getComputedStyle(document.documentElement).getPropertyValue(name).trim()

/* App.vue 用法：
import { darkTheme } from "naive-ui"
const isDark = ref(document.documentElement.dataset.theme === "dark")
<n-config-provider :theme="isDark ? darkTheme : null" :theme-overrides="isDark ? darkOverrides : lightOverrides">
*/
