import type * as React from "react";

declare module "react" {
  namespace JSX {
    interface IntrinsicElements {
      "stripe-buy-button": React.DetailedHTMLProps<React.HTMLAttributes<HTMLElement>, HTMLElement> & {
        "buy-button-id": string;
        "publishable-key": string;
      };
    }
  }
}
