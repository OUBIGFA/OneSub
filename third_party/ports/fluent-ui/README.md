# Fluent UI design reference

Reference: [Microsoft Fluent UI](https://github.com/microsoft/fluentui/tree/19e2d5cd2bf81a2406327709b8856242ec38a966), revision `19e2d5cd2bf81a2406327709b8856242ec38a966`, MIT. The license is retained in [fluent-ui.txt](../../licenses/fluent-ui.txt).

OneSub maps a small set of Fluent 2 neutral color values from [`packages/tokens/src/alias/darkColor.ts`](https://github.com/microsoft/fluentui/blob/19e2d5cd2bf81a2406327709b8856242ec38a966/packages/tokens/src/alias/darkColor.ts) to MyGo's native theme fields: canvas `#1f1f1f`, controls `#292929`, hover `#3d3d3d`, pressed `#383838`, content surfaces `#141414`, and secondary text `#adadad`. OneSub uses softened primary text `#efefef`, control borders `#525252`, and quieter region dividers `#343434`. OneSub retains its existing blue action/selection colors and accessible semantic colors.

The toolbar's grouping and density were reviewed against Fluent's [toolbar spacing](https://github.com/microsoft/fluentui/blob/19e2d5cd2bf81a2406327709b8856242ec38a966/packages/react-components/react-toolbar/library/src/components/Toolbar/useToolbarStyles.styles.ts). No Fluent React component, renderer, stylesheet, artwork or runtime is copied or bundled. All controls, layouts, focus states, menus and interactions remain MyGo native components.

The light palette continues to use the separately registered [WinOne colors](../winone/README.md).
