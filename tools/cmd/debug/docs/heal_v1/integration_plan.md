# Implementation Plan: Integrating URL Normalization and Routing Improvements

This document outlines the plan for integrating our URL normalization and routing improvements into the main codebase to resolve the "cannot route message" errors in the healing process.

## 1. Overview of Changes

We've developed several components to address the URL construction inconsistencies and routing conflicts:

1. **URL Normalization**: Functions to standardize URLs to use the consistent format from `sequence.go` (e.g., `acc://bvn-Apollo.acme`) rather than the anchor pool URL format (e.g., `acc://dn.acme/anchors/Apollo`).

2. **Normalizing Router**: A router wrapper that normalizes URLs before routing to ensure consistent routing decisions.

3. **Enhanced Submit Loop**: An improved version of the submission process that handles routing conflicts through URL normalization, caching, and direct addressing.

4. **Direct Router**: A router that uses direct addressing for problematic URLs through explicit overrides.

## 2. Integration Steps

### Step 1: Update `heal_common.go`

1. Replace the existing `submitLoop` function with our enhanced version:
   ```go
   // In heal_common.go
   func (h *healer) submitLoop(wg *sync.WaitGroup) {
       // Replace with the implementation from routing_implementation.go
   }
   ```

2. Add the `setupWithNormalizedRouting` function and modify the existing `setup` function to use it:
   ```go
   // In heal_common.go
   func (h *healer) setup(ctx context.Context, network string) {
       // Keep the original implementation
       
       // Add at the end:
       // Wrap the router with our normalizing router
       if h.router != nil {
           h.router = &NormalizingRouter{Router: h.router}
           slog.Info("Enhanced router with URL normalization")
       }
       
       // Update the client to use our normalizing router
       if c, ok := h.C2.Transport.(*message.RoutedTransport); ok {
           c.Router = &NormalizingRouter{Router: c.Router}
           slog.Info("Enhanced client transport with URL normalization")
       }
   }
   ```

### Step 2: Update `heal_anchor.go`

1. Update the `healAnchor` function to use normalized URLs:
   ```go
   // In heal_anchor.go
   func healAnchor(_ *cobra.Command, args []string) {
       lightDb = ""
       h := &healer{
           healSingle: func(h *healer, src, dst *protocol.PartitionInfo, num uint64, txid *url.TxID) {
               h.healSingleAnchorWithNormalizedUrls(src.ID, dst.ID, num, txid, nil)
           },
           healSequence: func(h *healer, src, dst *protocol.PartitionInfo) {
               // Skip BVN to BVN anchors
               if src.Type != protocol.PartitionTypeDirectory && dst.Type != protocol.PartitionTypeDirectory {
                   return
               }

               // Use normalized URLs
               srcUrl, dstUrl := normalizeAnchorUrl(src.ID, dst.ID)

           pullAgain:
               dstLedger := getAccount[*protocol.AnchorLedger](h, dstUrl.JoinPath(protocol.AnchorPool))
               src2dst := dstLedger.Partition(srcUrl)

               ids, txns := h.findPendingAnchors(srcUrl, dstUrl, true)

               var all []*url.TxID
               all = append(all, src2dst.Pending...)
               all = append(all, ids...)

               for i, txid := range all {
                   select {
                   case <-h.ctx.Done():
                       return
                   default:
                   }
                   if h.healSingleAnchorWithNormalizedUrls(src.ID, dst.ID, src2dst.Delivered+1+uint64(i), txid, txns) {
                       // If it was already delivered, recheck the ledgers
                       goto pullAgain
                   }
               }
           },
       }

       h.heal(args)
   }
   ```

2. Replace the existing `healSingleAnchor` function with our enhanced version:
   ```go
   // In heal_anchor.go
   func (h *healer) healSingleAnchor(srcId, dstId string, seqNum uint64, txid *url.TxID, txns map[[32]byte]*protocol.Transaction) bool {
       // Replace with the implementation from routing_implementation.go (healSingleAnchorWithNormalizedUrls)
   }
   ```

### Step 3: Add the New Components to the Codebase

1. Add the `NormalizingRouter` and `DirectRouter` implementations to the codebase:
   ```go
   // In heal_common.go or a new file like heal_router.go
   
   // NormalizingRouter wraps a Router and normalizes URLs before routing
   type NormalizingRouter struct {
       Router routing.Router
   }
   
   // RouteAccount normalizes the URL before routing to ensure consistent routing
   func (r *NormalizingRouter) RouteAccount(u *url.URL) (string, error) {
       normalized := normalizeUrl(u)
       return r.Router.RouteAccount(normalized)
   }
   
   // Route normalizes all URLs in the envelopes before routing
   func (r *NormalizingRouter) Route(envs ...*messaging.Envelope) (string, error) {
       // Implementation from routing_implementation.go
   }
   
   // DirectRouter implementation from routing_implementation.go
   ```

2. Add the URL normalization functions if not already present:
   ```go
   // In heal_common.go or a new file like heal_url.go
   
   // normalizeUrl implementation from url_normalization.go
   
   // normalizeUrlsInMessage implementation from url_normalization.go
   
   // normalizeAnchorUrl implementation from routing_implementation.go
   ```

## 3. Testing Plan

1. **Unit Tests**: Create unit tests for the URL normalization functions to ensure they correctly convert between different URL formats.

2. **Integration Tests**: Test the healing process with problematic anchors that previously failed due to routing conflicts.

3. **Manual Testing**: Manually verify that the healing process works correctly for different types of anchors and different URL formats.

## 4. Rollout Plan

1. **Development Environment**: Deploy the changes to a development environment and test thoroughly.

2. **Staging Environment**: Deploy to a staging environment and verify that the healing process works correctly.

3. **Production Environment**: Deploy to production after thorough testing.

## 5. Monitoring and Validation

1. **Logging**: Add detailed logging to track URL normalization and routing decisions.

2. **Metrics**: Track the number of routing conflicts and successful submissions.

3. **Alerts**: Set up alerts for persistent routing conflicts that cannot be resolved automatically.

## 6. Fallback Plan

If issues arise with the new implementation, we can temporarily revert to the original code while addressing any problems.

## 7. Documentation

Update the documentation to reflect the new URL normalization and routing improvements, including:

1. **Developer Guide**: Update with information about URL construction and routing.

2. **Troubleshooting Guide**: Add a section on resolving routing conflicts.

3. **API Documentation**: Update to reflect the new routing behavior.

## Conclusion

By implementing these changes, we will ensure consistent URL construction and routing throughout the codebase, resolving the "cannot route message" errors in the healing process. The URL normalization approach provides a clean solution that maintains backward compatibility while improving reliability.
