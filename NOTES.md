# The data model and why.
My data models are defined in [/pkg/store/models.go](/pkg/store/models.go).  I utilized `gorm` since I had prior work experience with it.  I utilized uniqueness constraints whenever needed, and I dicated foreign keys whenever there's an entity relations (like one-to-many or self-referencing).  Lessons to a path are ordered, and for this assessment, I wanted to demonstrate my knowledge and prior work experience with ordered list implementations.  See [Ordered List Implementation](#ordered-list-implementation)

# How you handle rules 3, 6, and 7.
Rule 3 dicates the need of an idempotency key.  Rule 6 and 7 was handled by Claude due to the time limit.

## Idempotency Key
I have experience with idempotency key with my prior work with digital prescription infrastructure.  Just like the learing events in this platform, a digital prescription sent from a doctor's clinic to a pharmacy must remain idempotent, meaning multiple duplicates of the same prescription must be deduplicated.  In this case, I used the database table for events as the idempotency key store.  Ideally, this store should be cached in memory for faster lookup.

# The decisions you made where the specification was silent, and why.

## Ordered List Implementation
I have personally worked on these three implementation of ordered list.  Each have their pros and cons.  Note that the app I have created has a flag `--sorting` that let you pick which sorting implementation to use.  See [/cmd/enrollments/main.go](/cmd/enrollments/main.go).

### Fractional Indexing
If you use `--sorting=0` (which is the default), the ordered list will use this implementation.  Fractional indexing implementation can be see in [/pkg/store/models.go](/pkg/store/models.go) with the use of `Index *float64` field and the sorting logic seen in [/pkg/operations/fractionalindexing.go](/pkg/operations/fractionalindexing.go)

I chose this as the go-to because the users in this application could create a high volume of re-orders.

Pros: Great for appication that has frequent re-ordering as a single-element reorder only requires updating one database record (reordering has a time complexity of O(1))

Cons: Floating point values have precision limits.  This can be mitigated by using high-precison floating point values that has at least 128 bits.  You can't tell the position of the record without collecting all the items in the collection and sorting them.

### Integer Position Column
If you use `--sorting=1`, the ordered list will use this implementation.  Integer position column implementation can be see in [/pkg/store/models.go](/pkg/store/models.go) with the use of `Order *uint` field and the sorting logic seen in [/pkg/operations/integerpositioncolumn.go](/pkg/operations/integerpositioncolumn.go)

Pros: Simplest implementation.  You can tell the position of an item with a query to a single database row.

Cons: Reordering is a very expensive task as it entails iterating through all the necessary elements to update all the integer position columns required.  If there are n elements in a collection, the time complexity to reorder one item is O(n).  This disadvantage is the main reason I did not chose this for this application.

### Adjacency List
If you use `--sorting=2`, the ordered list will use this implementation.  Adjacency list implementation can be see in [/pkg/store/models.go](/pkg/store/models.go) with the use of `NextId *int64` and `NextLesson *LessonRow` fields.  The sorting logic for this can be seen in [/pkg/operations/adjacencylist.go](/pkg/operations/adjacencylist.go)

Pros: Great for applications that frequently visits the adjacent element.  I use this in application that structures data in a DAG (directed acyclic graph) such as my prior work experience with XBRL data.

Cons: Reordering is not very expensive as it only requries update three database rows at the most.  However, if the list has to be present in a sorted manner, the sorting itself is very expensive (at time complexity of O(n^2)), which is why I did not use this approach.  I used this in XBRL where the data is rarely presented in a sorted manner and the user has the patience to wait for the app to render the sorted data.

## Scenario 11
The assignment explicitly dicates the use of an idempotency key, but was silent whether it should be "last-write-win" or "first-write-win".  I decided to go with "first-write-win".  In my prior work experience, sometimes the data should be "eventually consistent" (essentially BASE as opposed to ACID).  If eventual consistency was a requirement, then "last-write-win" would have been the way to go.  However, in this app, there was no clear requirement for "last-write-win", so I did not implement it.

## Scenario 12
I implemented what is known as "Postel's law" (aka robustness principle).  Per Postel's Law, I had the accept app all valid values as possible even if it is in a batch of bad value.

## Scenario 13
I decided to implement a "grandfather rule" purely to satisfy customers who I suspect would be disgruntled if they don't get grandfathered to a retired path.  Customer satisfaction was the main factor for my design decision here.

# What you would change if events arrived from a message broker at high volume.
Probably just the cache for the idempotency key store.  The fractional indexing is the best implementation for this application, but I might look for ways to use 128 bit floating point values for the indexing.

# What you cut for time.
Rule 6 and 7 was done by Claude as I was running out of time to implement all the operations by hand.  I also had Claude implement the sorting logic for adjacency list.  As you can see, I had Claude setup the database mapping, web UI, and docker compose.  See [CLAUDE.md](CLAUDE.md)

