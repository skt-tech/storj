# Storage Node Downtime Disqualification

## Abstract

This document describes storage node downtime suspension and disqualification.

## Background

The previous implementation of downtime disqualification led to the unfair disqualification of storage nodes. This prompted a halt to downtime disqualification and a new design for [Downtime Tracking](storage-node-downtime-tracking.md).

While the previously mentioned blueprint presents a means of tracking downtime, it leaves out the design of how to use this information for disqualification. That is the purpose of this blueprint.

## Design

### Suspension

Storage node downtime can have a range of causes. For those storage node operators who may have fallen victim to a temporary issue, we want to give them a chance to diagnose and fix it before disqualifying them for good. For this reason, we are introducing downtime suspension as a component of disqualification.

For downtime suspension and disqualification we need a few new configurable values:

- Tracking Period: The length of time into the past for which we measure downtime.
- Allowed Downtime: The amount of downtime within the tracking period we will allow before suspension.
- Grace Period: The length of time to give suspended SNOs to diagnose and fix issues causing downtime. Afterwards, they will have one tracking period to demonstrate an acceptable amount of downtime before disqualification.

When the [downtime tracking system](storage-node-downtime-tracking.md) adds an entry to the nodes_offline_time table, if the node is not already suspended, we check the total amount of downtime for the tracking period, e.g. if the tracking period is 30 days, we sum downtime for the last 30 days. If the total is greater than the allowed downtime, the node is suspended for a length of time equal to the grace period + one tracking period.

Suspended nodes are not selected for uploads, and all pieces they hold are considered unhealthy. 

### Disqualifying or reinstating suspended nodes

In order to be reinstated, suspended nodes need to prove to us that they have changed their behavior. This requires that we observe a period of acceptable downtime from them. Otherwise, they are disqualified.

For this purpose we implement a new chore for evaluating suspended nodes. The evaluation chore runs on a configurable interval, and checks to see if any downtime-suspended nodes have reached the end of their suspension (grace period + tracking period). For these nodes, check the total downtime for the tracking period. If a node's downtime has fallen within the allowed downtime range, it is reinstated. Otherwise, it is disqualified.

## Rationale

### Revert to using audits to track downtime?
This blueprint for disqualification was written to work with the currently implemented downtime tracking system. However, questions have recently been raised as to whether the change from using audits to check downtime to the current 'downtime tracking' system was necessary. The impetus for the transition was that a node could become disqualified by being offline for only a short period of time due to the chance that it could be audited many times in quick succession. That said, auditing does give us some indication of node downtime. Is implementing a new system for tracking downtime worth the additional resources if the audit system is already randomly reaching out to nodes? Ultimately what we care about is whether nodes are available to receive and deliver data and verifying that said data is intact. If a node is offline when audited, neither of these cases are true. Whether the node was offline for five minutes or five hours, it still failed to present the data when requested. However, it would be remiss of us to simply ignore the plight of the unlucky node. A certain amount of downtime is expected, and when, by chance, nodes can be disqualified within the expected window, it seems that time indeed is a factor which should be taken into account. Is it possible for us to salvage the previous system and account for its flaws? Perhaps. Here is a quick idea for discussion, the details of which would require further investigation:

Scale the _weight_ of the uptime check based on time elapsed since last contact.

The previous uptime reputation system worked much like the current audit reputation. See [Node Selection](node-selection.md) for more information.
> α(n) = λ·α(n-1) + _w_(1+_v_)/2
>
> β(n) = λ·β(n-1) + _w_(1-_v_)/2
>
> R(n) = α(n) / (α(n) + β(n))

In the case of a successful uptime check, _v_ is set to 1. Otherwise, it is set to -1. Previously, _w_, or _weight_, was configured at runtime and thereafter remained static. Thus, all uptime checks carried the same weight. However, should this really be the case?
For example, a node fails two uptime checks within a timespan of 1 minute. The second failure does not give us very much additional information does it? We already knew the node was offline less than one minute ago. If the difference were one hour that information might be more useful in determining uptime.

We might be able to account for this by adjusting _w_ based on how much time has elapsed since last contact.
To do this we'll need a new configurable `Uptime Scale`: a duration against which to scale the weight of an individual uptime check. To demonstrate, let's take an Uptime Scale of one hour. If a node is found to be offline when audited and its last contact is greater than or equal to one hour ago, the weight is scaled to 100%. This check will result in the full reputation punishment. A second failed check occurs 30 minutes later. This is one half the scale: the weight is reduced by 50%. Another failed check occurs one minute later, and the weight is scaled back to 1/60 the punishment.

### Tracking period
One might think to measure downtime within a static window, such as within month boundaries, after which downtime is reset. This introduces a problem. If one day of downtime within the tracking period is allowed, why _not_ shut down on the final day? This is compounded by the fact that any number of nodes could have the same idea, resulting in a potentially significant portion the network going offline at the same time. The trailing tracking period does not fall victim to the same problem. There is no specific timeframe which has a higher incentive for downtime, and any observed downtime will follow the node for the length of the tracking period.

### Early reinstatement for good nodes?
An alternate approach to suspending for a required length of time before reinstatement could be that we periodically evaluate a node's downtime during suspension. If the node's downtime has fallen within the acceptable range, we reinstate it. However, this could result in nodes falling in and out of suspension. If we suspend the node for a fixed period of time and evaluate its downtime at the end of the period, we can limit the rate of fluctuation.

## Implementation

1. Remove old uptime reputation values from codebase:

    uptime_success_count<br>
    total_uptime_count<br>
    uptime_reputation_alpha<br> 
    uptime_reputation_beta<br>

2. Add `downtime_suspended` column to the nodes table and rename the `suspended` column to `audit_suspended`

3. Implement new logic in estimation chore to suspend nodes
    - Refactor downtimeTrackingDB method `GetOfflineTime` to check for entries which contain downtime from outside the measurement period

        There is a particular characteristic of downtime tracking to take note of here:<br>
        
        A downtime entry tracked at March 2nd 00:00 contains downtime for some length of time preceding that point. See [downtime tracking](storage-node-downtime-tracking.md)<br>
        
        If we want to measure total downtime starting at March 1st 00:00, and there is a March 1st 01:00 entry indicating 2 hours of downtime, we need to truncate the measured downtime in our calculation in order to avoid unfairly including downtime which occurred outside the tracking period.<br>
        
        We should be able to solve this problem by taking the first entry within the tracking period and checking if the offline time it contains extends beyond the start of the tracking period. If so, truncate it to the start of the tracking period, then sum the rest of the entries as usual.

    - Refactor relevant overlay methods to handle downtime_suspended nodes: `KnownReliable`, `FindNodes`, etc.
    - Add notification to SN dashboard indicating suspension for downtime.

4. Implement evaluation chore to reinstate and disqualify nodes
    - If node.downtime_suspended <= now - (grace period + tracking period) it is eligible for evaluation
    - If the node is eligible for evaluation, we must also ensure that the entire tracking period is accounted for. If last contact was a failure and occurred before the end of the tracking period, we might have a window of downtime which has not yet been recorded. Thus, to measure all downtime for the tracking period, we must also wait until last_contact_success > failure OR last_contact_failure >= end of tracking period.

        When these conditions are met, measurements should take care to include all downtime within the period. In addition to the lower bound issue explained above under step 3, we also have an upper bound issue in this case. We may have an entry which exists beyond the upper bound of the tracking period, yet holds some amount of downtime which occured within it.<br>
        <br>
        For example, if we want to measure the total downtime for node A from March 1st 00:00 to March 31st 00:00, and we have an entry from April 1st 01:00 indicating 2 hours of downtime, we must include the one hour of downtime which occurred within the tracking period in our calculation. 

## Wrapup

- The person that implements step 4 above should archive this document.

- The [Disqualification blueprint](disqualification.md), and possibly the whitepaper, will need to be updated to reflect new up/downtime disqualification mechanic.

- Edit [Audit Suspension blueprint](audit-suspend.md) to reflect change from `suspended` to `audit_suspended`

- Link to this document in [Downtime Tracking](storage-node-downtime-tracking.md)

## Open issues

- It is possible for a node to continuously cycle through suspension and reinstatement. How frequently this could happen depends upon the length of the tracking and grace periods. Should there be a maximum number suspensions before disqualification?

